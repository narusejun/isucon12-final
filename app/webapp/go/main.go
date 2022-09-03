package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/bytedance/sonic"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jmoiron/sqlx"
	"github.com/mono0x/prand"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

var (
	ErrInvalidRequestBody       error = fmt.Errorf("invalid request body")
	ErrInvalidMasterVersion     error = fmt.Errorf("invalid master version")
	ErrInvalidItemType          error = fmt.Errorf("invalid item type")
	ErrInvalidToken             error = fmt.Errorf("invalid token")
	ErrGetRequestTime           error = fmt.Errorf("failed to get request time")
	ErrExpiredSession           error = fmt.Errorf("session expired")
	ErrUserNotFound             error = fmt.Errorf("not found user")
	ErrUserDeviceNotFound       error = fmt.Errorf("not found user device")
	ErrItemNotFound             error = fmt.Errorf("not found item")
	ErrLoginBonusRewardNotFound error = fmt.Errorf("not found login bonus reward")
	ErrNoFormFile               error = fmt.Errorf("no such file")
	ErrUnauthorized             error = fmt.Errorf("unauthorized user")
	ErrForbidden                error = fmt.Errorf("forbidden")
	ErrGeneratePassword         error = fmt.Errorf("failed to password hash") //nolint:deadcode
)

const (
	DeckCardNumber      int = 3
	PresentCountPerPage int = 100

	SQLDirectory string = "../sql/"
)

type Handler struct {
	snowflakeNode *snowflake.Node
}

func main() {
	rand.Seed(time.Now().UnixNano())
	time.Local = time.FixedZone("Local", 9*60*60)

	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	//runtime.GOMAXPROCS(1)

	app := fiber.New(fiber.Config{
		//Prefork:     true,
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
		Concurrency: 1000000,
	})

	//idx := 0
	//app.Hooks().OnFork(func(i int) error {
	//	err := exec.Command("/usr/bin/taskset", "-cp", strconv.Itoa(idx), strconv.Itoa(i)).Run()
	//	if err != nil {
	//		log.Printf("failed to taskset: %v", err)
	//		return err
	//	}
	//	idx++
	//
	//	return nil
	//})

	app.Hooks().OnShutdown(func() error {
		buf, err := json.Marshal(oneTimeTokenType1)
		if err != nil {
			return err
		}
		if err := os.WriteFile("user_one_time_tokens_type1.json", buf, 0777); err != nil {
			return err
		}
		buf, err = json.Marshal(oneTimeTokenType2)
		if err != nil {
			return err
		}
		if err := os.WriteFile("user_one_time_tokens_type2.json", buf, 0777); err != nil {
			return err
		}
		buf, err = json.Marshal(userSession)
		if err != nil {
			return err
		}
		if err := os.WriteFile("user_session.json", buf, 0777); err != nil {
			return err
		}
		return nil
	})

	if buf, err := os.ReadFile("user_one_time_tokens_type1.json"); err != nil {
		log.Printf("failed to read file: %v", err)
	} else {
		if err := json.Unmarshal(buf, &oneTimeTokenType1); err != nil {
			log.Printf("failed to unmarshal: %v", err)
		}
	}
	if buf, err := os.ReadFile("user_one_time_tokens_type2.json"); err != nil {
		log.Printf("failed to read file: %v", err)
	} else {
		if err := json.Unmarshal(buf, &oneTimeTokenType2); err != nil {
			log.Printf("failed to unmarshal: %v", err)
		}
	}
	if buf, err := os.ReadFile("user_session.json"); err != nil {
		log.Printf("failed to read file: %v", err)
	} else {
		if err := json.Unmarshal(buf, &userSession); err != nil {
			log.Printf("failed to unmarshal: %v", err)
		}
	}

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Content-Type, x-master-version, x-session",
	}))

	// Snowflake
	var nodeId int64
	if n, err := strconv.ParseInt(os.Getenv("ISUCON_SNOWFLAKE_NODE_ID"), 10, 64); err == nil {
		nodeId = n
	}
	snowflakeNode, err := snowflake.NewNode(nodeId)
	if err != nil {
		panic(err)
	}

	// init db
	if err := initDatabase(); err != nil {
		panic(err)
	}

	h := &Handler{
		snowflakeNode: snowflakeNode,
	}

	// utility
	app.Post("/initialize", initialize)
	app.Post("/initializeOne", initializeOne)
	app.Get("/health", h.health)

	// feature
	app.Post("/user", h.apiMiddleware, h.createUser)
	app.Post("/login", h.apiMiddleware, h.login)

	app.Get("/user/:userID/gacha/index", h.apiMiddleware, h.checkSessionMiddleware, h.listGacha)
	app.Post("/user/:userID/gacha/draw/:gachaID/:n", h.apiMiddleware, h.checkSessionMiddleware, h.drawGacha)
	app.Get("/user/:userID/present/index/:n", h.apiMiddleware, h.checkSessionMiddleware, h.listPresent)
	app.Post("/user/:userID/present/receive", h.apiMiddleware, h.checkSessionMiddleware, h.receivePresent)
	app.Get("/user/:userID/item", h.apiMiddleware, h.checkSessionMiddleware, h.listItem)
	app.Post("/user/:userID/card/addexp/:cardID", h.apiMiddleware, h.checkSessionMiddleware, h.addExpToCard)
	app.Post("/user/:userID/card", h.apiMiddleware, h.checkSessionMiddleware, h.updateDeck)
	app.Post("/user/:userID/reward", h.apiMiddleware, h.checkSessionMiddleware, h.reward)
	app.Get("/user/:userID/home", h.apiMiddleware, h.checkSessionMiddleware, h.home)

	// admin
	app.Post("/admin/login", h.adminMiddleware, h.adminLogin)

	app.Delete("/admin/logout", h.adminMiddleware, h.adminSessionCheckMiddleware, h.adminLogout)
	app.Get("/admin/master", h.adminMiddleware, h.adminSessionCheckMiddleware, h.adminListMaster)
	app.Put("/admin/master", h.adminMiddleware, h.adminSessionCheckMiddleware, h.adminUpdateMaster)
	app.Get("/admin/user/:userID", h.adminMiddleware, h.adminSessionCheckMiddleware, h.adminUser)
	app.Post("/admin/user/:userID/ban", h.adminMiddleware, h.adminSessionCheckMiddleware, h.adminBanUser)

	if _, err := forceRecache(adminDatabase()); err != nil {
		log.Fatal(err)
	}

	// setting server
	if getEnv("UNIX_DOMAIN_SOCKET", "") != "" {
		os.MkdirAll("/var/run", 0777)

		socket_file := "/var/run/app.sock"
		os.Remove(socket_file)

		l, err := net.Listen("unix", socket_file)
		if err != nil {
			log.Fatal(err)
		}

		// go runユーザとnginxのユーザ（グループ）を同じにすれば777じゃなくてok
		err = os.Chmod(socket_file, 0777)
		if err != nil {
			log.Fatal(err)
		}
		app.Listener(l)
	} else {
		app.Listen("0.0.0.0:" + getEnv("ISUCON_LISTEN_PORT", "8080"))
	}
}

// adminMiddleware
func (h *Handler) adminMiddleware(c *fiber.Ctx) error {
	requestAt := time.Now()
	c.Context().SetUserValue("requestTime", requestAt.Unix())

	// next
	return c.Next()
}

// apiMiddleware
func (h *Handler) apiMiddleware(c *fiber.Ctx) error {
	requestAt, err := time.Parse(time.RFC1123, c.Get("x-isu-date"))
	if err != nil {
		requestAt = time.Now()
	}
	c.Context().SetUserValue("requestTime", requestAt.Unix())

	userID, err := getUserID(c)
	if err != nil {
		userID = 0
	}

	// マスタ確認
	masterVersion, err := shouldRecache(selectDatabase(userID))
	if err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, fmt.Errorf("active master version is not found"))
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	if masterVersion != c.Get("x-master-version") {
		return errorResponse(c, http.StatusUnprocessableEntity, ErrInvalidMasterVersion)
	}

	// check ban
	if userID != 0 {
		isBan, err := h.checkBan(userID)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if isBan {
			return errorResponse(c, http.StatusForbidden, ErrForbidden)
		}
	}

	// next
	return c.Next()
}

// checkSessionMiddleware
var (
	userSession = make(map[int64][]*Session)
)

func (h *Handler) checkSessionMiddleware(c *fiber.Ctx) error {
	sessID := c.Get("x-session")
	if sessID == "" {
		return errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
	}

	sesssionUserIDStrs := strings.Split(sessID, "::")
	if len(sesssionUserIDStrs) != 2 {
		return errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
	}
	sesssionUserID, err := strconv.ParseInt(sesssionUserIDStrs[1], 10, 64)
	if err != nil {
		return errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
	}

	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if userID != sesssionUserID {
		return errorResponse(c, http.StatusForbidden, ErrForbidden)
	}

	if sessions, ok := userSession[sesssionUserID]; ok {
		ok := false
		userSession[sesssionUserID] = make([]*Session, 0)
		for i := 0; i < len(sessions); i++ {
			if sessions[i].ExpiredAt > requestAt {
				if sessions[i].SessionID == sessID {
					ok = true
				}
				userSession[sesssionUserID] = append(userSession[sesssionUserID], sessions[i])
			}
		}
		if !ok {
			return errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
		}
	} else {
		return errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
	}

	// next
	return c.Next()
}

// checkOneTimeToken
var (
	oneTimeTokenType1 = make(map[int64][]*UserOneTimeToken)
	oneTimeTokenType2 = make(map[int64][]*UserOneTimeToken)
)

func (h *Handler) checkOneTimeToken(userID int64, token string, tokenType int, requestAt int64) error {
	if tokenType == 1 {
		if tokens, ok := oneTimeTokenType1[userID]; ok {
			valid := false
			oneTimeTokenType1[userID] = make([]*UserOneTimeToken, 0)
			for i := 0; i < len(tokens); i++ {
				if tokens[i].Token == token && tokens[i].ExpiredAt > requestAt {
					valid = true
				} else if tokens[i].ExpiredAt > requestAt {
					oneTimeTokenType1[userID] = append(oneTimeTokenType1[userID], tokens[i])
				}
			}
			if valid {
				return nil
			}
		}
		return ErrInvalidToken
	} else if tokenType == 2 {
		if tokens, ok := oneTimeTokenType2[userID]; ok {
			valid := false
			oneTimeTokenType2[userID] = make([]*UserOneTimeToken, 0)
			for i := 0; i < len(tokens); i++ {
				if tokens[i].Token == token && tokens[i].ExpiredAt > requestAt {
					valid = true
				} else if tokens[i].ExpiredAt > requestAt {
					oneTimeTokenType2[userID] = append(oneTimeTokenType2[userID], tokens[i])
				}
			}
			if valid {
				// 使ったトークンを失効する
				return nil
			}
		}
		return ErrInvalidToken
	}

	tk := userOneTimeTokenPool.get()
	defer userOneTimeTokenPool.put(tk)
	query := "SELECT `expired_at` FROM user_one_time_tokens WHERE token=? AND token_type=? AND deleted_at IS NULL"
	if err := selectDatabase(userID).Get(tk, query, token, tokenType); err != nil {
		if err == sql.ErrNoRows {
			return ErrInvalidToken
		}
		return err
	}

	if tk.ExpiredAt < requestAt {
		query = "UPDATE user_one_time_tokens SET deleted_at=? WHERE token=?"
		if _, err := selectDatabase(userID).Exec(query, requestAt, token); err != nil {
			return err
		}
		return ErrInvalidToken
	}

	// 使ったトークンを失効する
	query = "UPDATE user_one_time_tokens SET deleted_at=? WHERE token=?"
	if _, err := selectDatabase(userID).Exec(query, requestAt, token); err != nil {
		return err
	}

	return nil
}

// checkViewerID
var (
	userDevices = make(map[int64]map[string]struct{})
)

func (h *Handler) checkViewerID(userID int64, viewerID string) error {
	if devices, ok := userDevices[userID]; ok {
		if _, ok := devices[viewerID]; ok {
			return nil
		}
	} else {
		userDevices[userID] = make(map[string]struct{})
	}
	query := "SELECT * FROM user_devices WHERE user_id=? AND platform_id=?"
	device := userDevicePool.get()
	defer userDevicePool.put(device)
	if err := selectDatabase(userID).Get(device, query, userID, viewerID); err != nil {
		if err == sql.ErrNoRows {
			return ErrUserDeviceNotFound
		}
		return err
	}

	userDevices[userID][viewerID] = struct{}{}

	return nil
}

// checkBan
var (
	userBan = make(map[int64]struct{})
)

func (h *Handler) checkBan(userID int64) (bool, error) {
	_, ok := userBan[userID]
	if ok {
		return false, nil
	}

	banUser := -1
	query := "SELECT 1 FROM user_bans WHERE user_id=?"
	if err := selectDatabase(userID).Get(&banUser, query, userID); err != nil {
		if err == sql.ErrNoRows {
			userBan[userID] = struct{}{}
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var (
	augEnd = int64(1661914800)
)

// getRequestTime リクエストを受けた時間をコンテキストからunixtimeで取得する
func getRequestTime(c *fiber.Ctx) (int64, error) {
	v := c.Context().UserValue("requestTime")
	if requestTime, ok := v.(int64); ok {
		return requestTime, nil
	}
	return 0, ErrGetRequestTime
}

// loginProcess ログイン処理
func (h *Handler) loginProcess(tx *sqlx.Tx, userID int64, requestAt int64) (*User, []*UserLoginBonus, []*UserPresent, error) {
	user := new(User)
	query := "SELECT * FROM users WHERE id=?"
	if err := tx.Get(user, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil, ErrUserNotFound
		}
		return nil, nil, nil, err
	}

	// ログインボーナス処理
	loginBonuses, err := h.obtainLoginBonus(tx, userID, requestAt)
	if err != nil {
		return nil, nil, nil, err
	}

	// 全員プレゼント取得
	allPresents, err := h.obtainPresent(tx, userID, requestAt)
	if err != nil {
		return nil, nil, nil, err
	}

	if err = tx.Get(&user.IsuCoin, "SELECT isu_coin FROM users WHERE id=?", user.ID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil, ErrUserNotFound
		}
		return nil, nil, nil, err
	}

	user.UpdatedAt = requestAt
	user.LastActivatedAt = requestAt

	query = "UPDATE users SET updated_at=?, last_activated_at=? WHERE id=?"
	if _, err := tx.Exec(query, requestAt, requestAt, userID); err != nil {
		return nil, nil, nil, err
	}

	return user, loginBonuses, allPresents, nil
}

// isCompleteTodayLogin ログイン処理が終わっているか
func isCompleteTodayLogin(lastActivatedAt, requestAt time.Time) bool {
	return lastActivatedAt.Year() == requestAt.Year() &&
		lastActivatedAt.Month() == requestAt.Month() &&
		lastActivatedAt.Day() == requestAt.Day()
}

var zeroUserLoginBonusArr = make([]*UserLoginBonus, 0)

// obtainLoginBonus
func (h *Handler) obtainLoginBonus(tx *sqlx.Tx, userID int64, requestAt int64) ([]*UserLoginBonus, error) {
	// login bonus masterから有効なログインボーナスを取得
	loginBonuses := loginBonusMasterPool.get()
	defer loginBonusMasterPool.put(loginBonuses)
	getLoginBonusMaster(requestAt, loginBonuses)
	if len(*loginBonuses) == 0 {
		return zeroUserLoginBonusArr, nil
	}

	// ボーナスの進捗を全取得
	loginBonusIds := int64ArrPool.get()
	defer int64ArrPool.put(loginBonusIds)
	for i := range *loginBonuses {
		*loginBonusIds = append(*loginBonusIds, (*loginBonuses)[i].ID)
	}
	query := "SELECT * FROM user_login_bonuses WHERE user_id=? AND login_bonus_id IN (?)"
	query, params, err := sqlx.In(query, userID, *loginBonusIds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	_progress := userLoginBonusArrPool.get()
	defer userLoginBonusArrPool.put(_progress)
	if err := tx.Select(_progress, query, params...); err != nil {
		return nil, errors.WithStack(err)
	}
	progress := make(map[int64]*UserLoginBonus, len(*_progress))
	//for _, bonus := range _progress {
	for i := 0; i < len(*_progress); i++ {
		progress[(*_progress)[i].LoginBonusID] = (*_progress)[i]
	}

	initBonuses := userLoginBonusArrPool.get()
	defer userLoginBonusArrPool.put(initBonuses)
	progressBonuses := userLoginBonusArrPool.get()
	defer userLoginBonusArrPool.put(progressBonuses)
	sendLoginBonuses := make([]*UserLoginBonus, 0)
	rewards := loginBonusRewardMasterArrPool.get()
	defer loginBonusRewardMasterArrPool.put(rewards)

	//for _, bonus := range loginBonuses {
	for i := 0; i < len((*loginBonuses)); i++ {
		userBonus := progress[(*loginBonuses)[i].ID]
		if userBonus == nil {
			ubID, err := h.generateID()
			if err != nil {
				return nil, errors.WithStack(err)
			}
			userBonus = &UserLoginBonus{ // ボーナス初期化
				ID:                 ubID,
				UserID:             userID,
				LoginBonusID:       (*loginBonuses)[i].ID,
				LastRewardSequence: 1,
				LoopCount:          1,
				CreatedAt:          requestAt,
				UpdatedAt:          requestAt,
			}
			*initBonuses = append(*initBonuses, userBonus)
		} else {
			// ボーナス進捗更新
			if userBonus.LastRewardSequence < (*loginBonuses)[i].ColumnCount {
				userBonus.LastRewardSequence++
			} else {
				if (*loginBonuses)[i].Looped {
					userBonus.LoopCount += 1
					userBonus.LastRewardSequence = 1
				} else {
					// 上限まで付与完了
					continue
				}
			}
			userBonus.UpdatedAt = requestAt
			*progressBonuses = append(*progressBonuses, userBonus)
		}
		sendLoginBonuses = append(sendLoginBonuses, userBonus)

		// 今回付与するリソース取得
		ok, rewardItem := getLoginBonusRewardMasterByIDAndSequence((*loginBonuses)[i].ID, userBonus.LastRewardSequence)
		if !ok {
			return nil, ErrLoginBonusRewardNotFound
		}
		*rewards = append(*rewards, &rewardItem)
	}

	if len(*initBonuses) > 0 {
		query = "INSERT INTO user_login_bonuses(id, user_id, login_bonus_id, last_reward_sequence, loop_count, created_at, updated_at) VALUES (:id, :user_id, :login_bonus_id, :last_reward_sequence, :loop_count, :created_at, :updated_at)"
		if _, err = tx.NamedExec(query, *initBonuses); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if len(*progressBonuses) > 0 {
		query = "UPDATE user_login_bonuses SET last_reward_sequence=?, loop_count=?, updated_at=? WHERE id=?"
		//for _, userBonus := range progressBonuses {
		for i := 0; i < len(*progressBonuses); i++ {
			if _, err = tx.Exec(query, (*progressBonuses)[i].LastRewardSequence, (*progressBonuses)[i].LoopCount, (*progressBonuses)[i].UpdatedAt, (*progressBonuses)[i].ID); err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}
	if len(*rewards) > 0 {
		var (
			coinAmount int64
			cardIDs    []int64
			item45s    []obtain45Item
		)
		for i := range *rewards {
			switch (*rewards)[i].ItemType {
			case 1: // coin
				coinAmount += (*rewards)[i].Amount
			case 2: // card
				cardIDs = append(cardIDs, (*rewards)[i].ItemID)
			default:
				item45s = append(item45s, obtain45Item{
					itemID:       (*rewards)[i].ItemID,
					obtainAmount: (*rewards)[i].Amount,
				})
			}
		}

		if err := h.obtainCoin(tx, userID, coinAmount); err != nil {
			return nil, errors.WithStack(err)
		}
		if err := h.obtainCards(tx, userID, requestAt, cardIDs); err != nil {
			return nil, errors.WithStack(err)
		}
		if err := h.obtain45Items(tx, userID, requestAt, item45s); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return sendLoginBonuses, nil
}

var zeroUserPresentArr = make([]*UserPresent, 0)

// obtainPresent プレゼント付与処理
func (h *Handler) obtainPresent(tx *sqlx.Tx, userID int64, requestAt int64) ([]*UserPresent, error) {
	normalPresents := presentAllMasterPool.get()
	defer presentAllMasterPool.put(normalPresents)
	getPresentAllMaster(requestAt, normalPresents)
	if len(*normalPresents) == 0 {
		return zeroUserPresentArr, nil
	}

	// 全員プレゼント取得情報更新
	ids := make([]int64, len(*normalPresents))
	for i := range *normalPresents {
		ids[i] = (*normalPresents)[i].ID
	}

	received := userPresentAllReceivedHistoryArrPool.get()
	defer userPresentAllReceivedHistoryArrPool.put(received)
	q, params, err := sqlx.In("SELECT present_all_id FROM user_present_all_received_history WHERE user_id=? AND present_all_id IN (?)", userID, ids)
	if err != nil {
		return nil, err
	}
	if err := tx.Select(received, q, params...); err != nil {
		return nil, err
	}
	receivedIds := make(map[int64]bool, len(*received))
	for i := range *received {
		receivedIds[(*received)[i].PresentAllID] = true
	}

	obtainPresents := make([]*UserPresent, 0)
	history := userPresentAllReceivedHistoryArrPool.get()
	defer userPresentAllReceivedHistoryArrPool.put(history)
	//for _, np := range normalPresents {
	for i := 0; i < len(*normalPresents); i++ {
		if receivedIds[(*normalPresents)[i].ID] {
			continue
		}

		// user present boxに入れる
		// historyに入れる
		pID, err := h.generateID()
		if err != nil {
			return nil, err
		}
		phID, err := h.generateID()
		if err != nil {
			return nil, err
		}
		up := &UserPresent{
			ID:             pID,
			UserID:         userID,
			SentAt:         requestAt,
			ItemType:       (*normalPresents)[i].ItemType,
			ItemID:         (*normalPresents)[i].ItemID,
			Amount:         int((*normalPresents)[i].Amount),
			PresentMessage: (*normalPresents)[i].PresentMessage,
			CreatedAt:      requestAt,
			UpdatedAt:      requestAt,
		}
		h := &UserPresentAllReceivedHistory{
			ID:           phID,
			UserID:       userID,
			PresentAllID: (*normalPresents)[i].ID,
			ReceivedAt:   requestAt,
			CreatedAt:    requestAt,
			UpdatedAt:    requestAt,
		}
		obtainPresents = append(obtainPresents, up)
		*history = append(*history, h)
	}

	if len(obtainPresents) == 0 {
		return obtainPresents, nil
	}

	_, err = tx.NamedExec(
		"INSERT INTO user_presents(id, user_id, sent_at, item_type, item_id, amount, present_message, created_at, updated_at) VALUES (:id, :user_id, :sent_at, :item_type, :item_id, :amount, :present_message, :created_at, :updated_at)",
		obtainPresents,
	)
	if err != nil {
		return nil, err
	}
	_, err = tx.NamedExec(
		"INSERT INTO user_present_all_received_history(id, user_id, present_all_id, received_at, created_at, updated_at) VALUES (:id, :user_id, :present_all_id, :received_at, :created_at, :updated_at)",
		*history,
	)
	if err != nil {
		return nil, err
	}

	return obtainPresents, nil
}

func (h *Handler) obtainCoin(tx *sqlx.Tx, userID, obtainAmount int64) error {
	_, err := tx.Exec("UPDATE users SET isu_coin=isu_coin+? WHERE id=?", obtainAmount, userID)
	return err
}

// itemIDsは重複する場合がある
func (h *Handler) obtainCards(tx *sqlx.Tx, userID, requestAt int64, itemIDs []int64) error {
	if len(itemIDs) == 0 {
		return nil
	}

	items := itemMasterArrPool.get()
	defer itemMasterArrPool.put(items)
	itemAmountPerSecMap := make(map[int64]int, len(*items))
	for _, itemID := range itemIDs {
		ok, item := getItemMasterByID(itemID)
		if !ok {
			continue
		}
		itemAmountPerSecMap[item.ID] = *item.AmountPerSec
	}

	cards := userCardArrPool.get()
	defer userCardArrPool.put(cards)
	for _, id := range itemIDs {
		cID, err := h.generateID()
		if err != nil {
			return err
		}
		*cards = append(*cards, &UserCard{
			ID:           cID,
			UserID:       userID,
			CardID:       id,
			AmountPerSec: itemAmountPerSecMap[id],
			Level:        1,
			TotalExp:     0,
			CreatedAt:    requestAt,
			UpdatedAt:    requestAt,
		})
	}

	q := "INSERT INTO user_cards(id, user_id, card_id, amount_per_sec, level, total_exp, created_at, updated_at) VALUES (:id, :user_id, :card_id, :amount_per_sec, :level, :total_exp, :created_at, :updated_at)"
	_, err := tx.NamedExec(q, *cards)
	return err
}

type obtain45Item struct {
	itemID       int64
	obtainAmount int64
}

func (h *Handler) obtain45Items(tx *sqlx.Tx, userID, requestAt int64, items []obtain45Item) error {
	if len(items) == 0 {
		return nil
	}
	itemIDs := make([]int64, len(items))
	itemMasters := make(map[int64]ItemMaster, 0)
	for idx, item := range items {
		itemIDs[idx] = item.itemID

		ok, i := getItemMasterByID(item.itemID)
		if !ok {
			continue
		}
		itemMasters[item.itemID] = i
	}

	q := "SELECT id, item_id, amount FROM user_items WHERE user_id=? AND item_id IN (?)"
	q, params, err := sqlx.In(q, userID, itemIDs)
	if err != nil {
		return err
	}
	_userItems := userItemsArrPool.get()
	defer userItemsArrPool.put(_userItems)
	if err := tx.Select(_userItems, q, params...); err != nil {
		return err
	}
	userItems := make(map[int64]*UserItem, len(*_userItems))
	for _, item := range *_userItems {
		userItems[item.ItemID] = item
	}

	blkInserts := userItemsArrPool.get()
	defer userItemsArrPool.put(blkInserts)
	blkUpdates := make(map[int64]int)
	for _, tmp := range items {
		if userItems[tmp.itemID] == nil {
			// 新規作成
			uitemID, err := h.generateID()
			if err != nil {
				return err
			}
			item := itemMasters[tmp.itemID]
			*blkInserts = append(*blkInserts, &UserItem{
				ID:        uitemID,
				UserID:    userID,
				ItemType:  item.ItemType,
				ItemID:    item.ID,
				Amount:    int(tmp.obtainAmount),
				CreatedAt: requestAt,
				UpdatedAt: requestAt,
			})
		} else { // 更新
			blkUpdates[tmp.itemID] += int(tmp.obtainAmount)
		}
	}

	if len(*blkInserts) > 0 {
		q = "INSERT INTO user_items(id, user_id, item_id, item_type, amount, created_at, updated_at) VALUES (:id, :user_id, :item_id, :item_type, :amount, :created_at, :updated_at)"
		_, err = tx.NamedExec(q, *blkInserts)
		if err != nil {
			return err
		}
	}
	if len(blkUpdates) > 0 {
		for id, amount := range blkUpdates {
			query := "UPDATE user_items SET amount=?, updated_at=? WHERE id=?"
			if _, err := tx.Exec(query, userItems[id].Amount+amount, requestAt, userItems[id].ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// initialize 初期化処理
// POST /initialize
var (
	inChecking = false
)

func initialize(c *fiber.Ctx) error {
	errCh := make(chan error, len(dbHosts))
	wg := sync.WaitGroup{}

	defer close(errCh)

	for _, host := range dbHosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()

			resp, err := http.Post(fmt.Sprintf("http://%s:8080/initializeOne", host), "application/json", nil)
			if err != nil {
				errCh <- err
				return
			}

			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("CODE: %d", resp.StatusCode)
				return
			}
		}(host)
	}

	wg.Wait()
	if len(errCh) > 0 {
		return errorResponse(c, http.StatusInternalServerError, <-errCh)
	}

	_, err := forceRecache(adminDatabase())
	if err != nil {
		log.Printf("Failed to recache masters : %v", err)
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	userDevices = make(map[int64]map[string]struct{})

	oneTimeTokenType1 = make(map[int64][]*UserOneTimeToken)
	oneTimeTokenType2 = make(map[int64][]*UserOneTimeToken)

	userSession = make(map[int64][]*Session)

	inChecking = true
	go func() {
		time.Sleep(1 * time.Second)
		inChecking = false
	}()

	return successResponse(c, &InitializeResponse{
		Language: "go",
	})
}
func initializeOne(c *fiber.Ctx) error {
	out, err := exec.Command("/bin/sh", "-c", SQLDirectory+"init.sh").CombinedOutput()
	if err != nil {
		log.Printf("Failed to initialize %s: %v", string(out), err)
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &InitializeResponse{
		Language: "go",
	})
}

type InitializeResponse struct {
	Language string `json:"language"`
}

// createUser ユーザの作成
// POST /user
func (h *Handler) createUser(c *fiber.Ctx) error {
	// parse body
	req := createUserRequestPool.get()
	defer createUserRequestPool.put(req)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	if req.ViewerID == "" || req.PlatformType < 1 || req.PlatformType > 3 {
		return errorResponse(c, http.StatusBadRequest, ErrInvalidRequestBody)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	// ユーザ作成
	uID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	tx, err := selectDatabase(uID).Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	defer tx.Rollback() //nolint:errcheck

	user := &User{
		ID:              uID,
		IsuCoin:         0,
		LastGetRewardAt: requestAt,
		LastActivatedAt: requestAt,
		RegisteredAt:    requestAt,
		CreatedAt:       requestAt,
		UpdatedAt:       requestAt,
	}
	query := "INSERT INTO users(id, last_activated_at, registered_at, last_getreward_at, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)"
	if _, err = tx.Exec(query, user.ID, user.LastActivatedAt, user.RegisteredAt, user.LastGetRewardAt, user.CreatedAt, user.UpdatedAt); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	udID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	userDevice := &UserDevice{
		ID:           udID,
		UserID:       user.ID,
		PlatformID:   req.ViewerID,
		PlatformType: req.PlatformType,
		CreatedAt:    requestAt,
		UpdatedAt:    requestAt,
	}
	query = "INSERT INTO user_devices(id, user_id, platform_id, platform_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"
	_, err = tx.Exec(query, userDevice.ID, user.ID, req.ViewerID, req.PlatformType, requestAt, requestAt)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	if _, ok := userDevices[user.ID]; !ok {
		userDevices[user.ID] = make(map[string]struct{})
	}
	userDevices[user.ID][req.ViewerID] = struct{}{}

	// 初期デッキ付与
	ok, initCard := getItemMasterByID(2)
	if !ok {
		return errorResponse(c, http.StatusInternalServerError, ErrItemNotFound)
	}

	initCards := make([]*UserCard, 0, 3)
	for i := 0; i < 3; i++ {
		cID, err := h.generateID()
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		card := &UserCard{
			ID:           cID,
			UserID:       user.ID,
			CardID:       initCard.ID,
			AmountPerSec: *initCard.AmountPerSec,
			Level:        1,
			TotalExp:     0,
			CreatedAt:    requestAt,
			UpdatedAt:    requestAt,
		}
		initCards = append(initCards, card)
	}
	query = "INSERT INTO user_cards(id, user_id, card_id, amount_per_sec, level, total_exp, created_at, updated_at) VALUES (:id, :user_id, :card_id, :amount_per_sec, :level, :total_exp, :created_at, :updated_at)"
	if _, err := tx.NamedExec(query, initCards); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	deckID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	initDeck := &UserDeck{
		ID:        deckID,
		UserID:    user.ID,
		CardID1:   initCards[0].ID,
		CardID2:   initCards[1].ID,
		CardID3:   initCards[2].ID,
		CreatedAt: requestAt,
		UpdatedAt: requestAt,
	}
	query = "INSERT INTO user_decks(id, user_id, user_card_id_1, user_card_id_2, user_card_id_3, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
	if _, err := tx.Exec(query, initDeck.ID, initDeck.UserID, initDeck.CardID1, initDeck.CardID2, initDeck.CardID3, initDeck.CreatedAt, initDeck.UpdatedAt); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// ログイン処理
	user, loginBonuses, presents, err := h.loginProcess(tx, user.ID, requestAt)
	if err != nil {
		if err == ErrUserNotFound || err == ErrItemNotFound || err == ErrLoginBonusRewardNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		if err == ErrInvalidItemType {
			return errorResponse(c, http.StatusBadRequest, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// generate session
	sID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	sessID, err := generateUUID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	sess := &Session{
		ID:        sID,
		UserID:    user.ID,
		SessionID: fmt.Sprintf("%s::%d", sessID, user.ID),
		CreatedAt: requestAt,
		UpdatedAt: requestAt,
		ExpiredAt: requestAt + 86400,
	}

	if _, ok := userSession[user.ID]; !ok {
		userSession[user.ID] = make([]*Session, 0)
	}
	userSession[user.ID] = append(userSession[user.ID], sess)

	//query = "INSERT INTO user_sessions(id, user_id, session_id, created_at, updated_at, expired_at) VALUES (?, ?, ?, ?, ?, ?)"
	//if _, err = tx.Exec(query, sess.ID, sess.UserID, sess.SessionID, sess.CreatedAt, sess.UpdatedAt, sess.ExpiredAt); err != nil {
	//	return errorResponse(c, http.StatusInternalServerError, err)
	//}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &CreateUserResponse{
		UserID:           user.ID,
		ViewerID:         req.ViewerID,
		SessionID:        sess.SessionID,
		CreatedAt:        requestAt,
		UpdatedResources: makeUpdatedResources(requestAt, user, userDevice, initCards, []*UserDeck{initDeck}, nil, loginBonuses, presents),
	})
}

type CreateUserRequest struct {
	ViewerID     string `json:"viewerId"`
	PlatformType int    `json:"platformType"`
}

type CreateUserResponse struct {
	UserID           int64            `json:"userId"`
	ViewerID         string           `json:"viewerId"`
	SessionID        string           `json:"sessionId"`
	CreatedAt        int64            `json:"createdAt"`
	UpdatedResources *UpdatedResource `json:"updatedResources"`
}

// login ログイン
// POST /login
func (h *Handler) login(c *fiber.Ctx) error {
	req := loginRequestPool.get()
	defer loginRequestPool.put(req)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	user := userPool.get()
	defer userPool.put(user)
	query := "SELECT * FROM users WHERE id=?"
	if err := selectDatabase(req.UserID).Get(user, query, req.UserID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, ErrUserNotFound)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// check ban
	isBan, err := h.checkBan(user.ID)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	if isBan {
		return errorResponse(c, http.StatusForbidden, ErrForbidden)
	}

	// viewer id check
	if err = h.checkViewerID(user.ID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	tx, err := selectDatabase(req.UserID).Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// sessionを更新
	//query = "UPDATE user_sessions SET deleted_at=? WHERE user_id=? AND deleted_at IS NULL"
	//if _, err = tx.Exec(query, requestAt, req.UserID); err != nil {
	//	return errorResponse(c, http.StatusInternalServerError, err)
	//}
	sID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	sessID, err := generateUUID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	sess := &Session{
		ID:        sID,
		UserID:    req.UserID,
		SessionID: fmt.Sprintf("%s::%d", sessID, user.ID),
		CreatedAt: requestAt,
		UpdatedAt: requestAt,
		ExpiredAt: requestAt + 86400,
	}

	//query = "INSERT INTO user_sessions(id, user_id, session_id, created_at, updated_at, expired_at) VALUES (?, ?, ?, ?, ?, ?)"
	//if _, err = tx.Exec(query, sess.ID, sess.UserID, sess.SessionID, sess.CreatedAt, sess.UpdatedAt, sess.ExpiredAt); err != nil {
	//	return errorResponse(c, http.StatusInternalServerError, err)
	//}
	userSession[req.UserID] = []*Session{sess}

	// すでにログインしているユーザはログイン処理をしない
	if isCompleteTodayLogin(time.Unix(user.LastActivatedAt, 0), time.Unix(requestAt, 0)) {
		user.UpdatedAt = requestAt
		user.LastActivatedAt = requestAt

		query = "UPDATE users SET updated_at=?, last_activated_at=? WHERE id=?"
		if _, err := tx.Exec(query, requestAt, requestAt, req.UserID); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}

		err = tx.Commit()
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}

		return successResponse(c, &LoginResponse{
			ViewerID:         req.ViewerID,
			SessionID:        sess.SessionID,
			UpdatedResources: makeUpdatedResources(requestAt, user, nil, nil, nil, nil, nil, nil),
		})
	}

	// login process
	user2, loginBonuses, presents, err := h.loginProcess(tx, req.UserID, requestAt)
	if err != nil {
		if err == ErrUserNotFound || err == ErrItemNotFound || err == ErrLoginBonusRewardNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		if err == ErrInvalidItemType {
			return errorResponse(c, http.StatusBadRequest, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &LoginResponse{
		ViewerID:         req.ViewerID,
		SessionID:        sess.SessionID,
		UpdatedResources: makeUpdatedResources(requestAt, user2, nil, nil, nil, nil, loginBonuses, presents),
	})
}

type LoginRequest struct {
	ViewerID string `json:"viewerId"`
	UserID   int64  `json:"userId"`
}

type LoginResponse struct {
	ViewerID         string           `json:"viewerId"`
	SessionID        string           `json:"sessionId"`
	UpdatedResources *UpdatedResource `json:"updatedResources"`
}

// listGacha ガチャ一覧
// GET /user/{userID}/gacha/index
func (h *Handler) listGacha(c *fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	gachaMasterList := gachaMasterPool.get()
	defer gachaMasterPool.put(gachaMasterList)
	getGachaMaster(requestAt, gachaMasterList)

	if len(*gachaMasterList) == 0 {
		return successResponse(c, &ListGachaResponse{
			Gachas: []*GachaData{},
		})
	}

	// ガチャ排出アイテム取得
	gachaDataList := make([]*GachaData, 0)
	for _, v := range *gachaMasterList {
		gachaItem := gachaItemMasterPool.get()
		defer gachaItemMasterPool.put(gachaItem)
		getGachaItemMasterByID(v.ID, gachaItem)

		if len(*gachaItem) == 0 {
			return errorResponse(c, http.StatusNotFound, fmt.Errorf("not found gacha item"))
		}

		gachaDataList = append(gachaDataList, &GachaData{
			Gacha:     v,
			GachaItem: *gachaItem,
		})
	}

	// genearte one time token
	tID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	tk, err := generateUUID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	token := &UserOneTimeToken{
		ID:        tID,
		UserID:    userID,
		Token:     tk,
		TokenType: 1,
		CreatedAt: requestAt,
		UpdatedAt: requestAt,
		ExpiredAt: requestAt + 600,
	}
	oneTimeTokenType1[userID] = []*UserOneTimeToken{token}
	delete(oneTimeTokenType2, userID)

	//go func() {
	//	query := "UPDATE user_one_time_tokens SET deleted_at=? WHERE user_id=? AND deleted_at IS NULL"
	//	selectDatabase(userID).Exec(query, requestAt, userID)
	//	query = "INSERT INTO user_one_time_tokens(id, user_id, token, token_type, created_at, updated_at, expired_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
	//	selectDatabase(userID).Exec(query, token.ID, token.UserID, token.Token, token.TokenType, token.CreatedAt, token.UpdatedAt, token.ExpiredAt)
	//}()

	return successResponse(c, &ListGachaResponse{
		OneTimeToken: token.Token,
		Gachas:       gachaDataList,
	})
}

type ListGachaResponse struct {
	OneTimeToken string       `json:"oneTimeToken"`
	Gachas       []*GachaData `json:"gachas"`
}

type GachaData struct {
	Gacha     *GachaMaster       `json:"gacha"`
	GachaItem []*GachaItemMaster `json:"gachaItemList"`
}

// drawGacha ガチャを引く
// POST /user/{userID}/gacha/draw/{gachaID}/{n}
func (h *Handler) drawGacha(c *fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	gachaID := c.Params("gachaID")
	if gachaID == "" {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid gachaID"))
	}

	gachaCount, err := strconv.ParseInt(c.Params("n"), 10, 64)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}
	if gachaCount != 1 && gachaCount != 10 {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid draw gacha times"))
	}

	req := drawGachaRequestPool.get()
	defer drawGachaRequestPool.put(req)
	if err = parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if err = h.checkOneTimeToken(userID, req.OneTimeToken, 1, requestAt); err != nil {
		if err == ErrInvalidToken {
			return errorResponse(c, http.StatusBadRequest, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	if err = h.checkViewerID(userID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	consumedCoin := int64(gachaCount * 1000)

	// userのisuconが足りるか
	user := userPool.get()
	defer userPool.put(user)
	query := "SELECT * FROM users WHERE id=?"
	if err := selectDatabase(userID).Get(user, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, ErrUserNotFound)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	if user.IsuCoin < consumedCoin {
		return errorResponse(c, http.StatusConflict, fmt.Errorf("not enough isucon"))
	}

	// gachaIDからガチャマスタの取得
	gachaIDint64, err := strconv.ParseInt(gachaID, 10, 64)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}
	ok, gachaInfo := getGachaMasterByID(gachaIDint64, requestAt)
	if !ok {
		return errorResponse(c, http.StatusNotFound, fmt.Errorf("not found gacha"))
	}

	// gachaItemMasterからアイテムリスト取得
	gachaIDint64, err = strconv.ParseInt(gachaID, 10, 64)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	gachaItemList := gachaItemMasterPool.get()
	defer gachaItemMasterPool.put(gachaItemList)
	getGachaItemMasterByID(gachaIDint64, gachaItemList)
	if len(*gachaItemList) == 0 {
		return errorResponse(c, http.StatusNotFound, fmt.Errorf("not found gacha item"))
	}

	// weightの合計値を算出
	var sum int64
	for _, gachaItem := range *gachaItemList {
		sum += int64(gachaItem.Weight)
	}

	// random値の導出 & 抽選
	result := gachaItemMasterPool.get()
	defer gachaItemMasterPool.put(result)
	for i := 0; i < int(gachaCount); i++ {
		random := prand.Int63n(sum)
		boundary := 0
		for _, v := range *gachaItemList {
			boundary += v.Weight
			if random < int64(boundary) {
				*result = append(*result, v)
				break
			}
		}
	}

	tx, err := selectDatabase(userID).Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// 直付与 => プレゼントに入れる
	presents := userPresentArrPool.get()
	defer userPresentArrPool.put(presents)
	for _, v := range *result {
		pID, err := h.generateID()
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		present := &UserPresent{
			ID:             pID,
			UserID:         userID,
			SentAt:         requestAt,
			ItemType:       v.ItemType,
			ItemID:         v.ItemID,
			Amount:         v.Amount,
			PresentMessage: fmt.Sprintf("%sの付与アイテムです", gachaInfo.Name),
			CreatedAt:      requestAt,
			UpdatedAt:      requestAt,
		}
		*presents = append(*presents, present)
	}

	query = "INSERT INTO user_presents(id, user_id, sent_at, item_type, item_id, amount, present_message, created_at, updated_at) VALUES (:id, :user_id, :sent_at, :item_type, :item_id, :amount, :present_message, :created_at, :updated_at)"
	if _, err := tx.NamedExec(query, *presents); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// isuconをへらす
	query = "UPDATE users SET isu_coin=? WHERE id=?"
	totalCoin := user.IsuCoin - consumedCoin
	if _, err := tx.Exec(query, totalCoin, user.ID); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &DrawGachaResponse{
		Presents: *presents,
	})
}

type DrawGachaRequest struct {
	ViewerID     string `json:"viewerId"`
	OneTimeToken string `json:"oneTimeToken"`
}

type DrawGachaResponse struct {
	Presents []*UserPresent `json:"presents"`
}

// listPresent プレゼント一覧
// GET /user/{userID}/present/index/{n}
func (h *Handler) listPresent(c *fiber.Ctx) error {
	n, err := strconv.Atoi(c.Params("n"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid index number (n) parameter"))
	}
	if n == 0 {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("index number (n) should be more than or equal to 1"))
	}

	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid userID parameter"))
	}

	offset := PresentCountPerPage * (n - 1)
	presentList := userPresentArrPool.get()
	defer userPresentArrPool.put(presentList)
	query := `
	SELECT * FROM user_presents
	WHERE user_id = ? AND deleted_at IS NULL
	ORDER BY created_at DESC, id
	LIMIT ? OFFSET ?`
	if err = selectDatabase(userID).Select(presentList, query, userID, PresentCountPerPage+1, offset); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	isNext := len(*presentList) == PresentCountPerPage+1
	if isNext {
		*presentList = (*presentList)[:PresentCountPerPage]
	}
	return successResponse(c, &ListPresentResponse{
		Presents: *presentList,
		IsNext:   isNext,
	})
}

type ListPresentResponse struct {
	Presents []*UserPresent `json:"presents"`
	IsNext   bool           `json:"isNext"`
}

// receivePresent プレゼント受け取り
// POST /user/{userID}/present/receive
func (h *Handler) receivePresent(c *fiber.Ctx) error {
	// read body
	req := receivePresentRequestPool.get()
	defer receivePresentRequestPool.put(req)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if len(req.PresentIDs) == 0 {
		return errorResponse(c, http.StatusUnprocessableEntity, fmt.Errorf("presentIds is empty"))
	}

	if err = h.checkViewerID(userID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// user_presentsに入っているが未取得のプレゼント取得
	query := "SELECT id, user_id, sent_at, item_type, item_id, amount, present_message, created_at FROM user_presents WHERE id IN (?) AND deleted_at IS NULL"
	query, params, err := sqlx.In(query, req.PresentIDs)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}
	obtainPresent := userPresentArrPool.get()
	defer userPresentArrPool.put(obtainPresent)
	if err = selectDatabase(userID).Select(obtainPresent, query, params...); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	if len(*obtainPresent) == 0 {
		return successResponse(c, &ReceivePresentResponse{
			UpdatedResources: makeUpdatedResources(requestAt, nil, nil, nil, nil, nil, nil, zeroUserPresentArr),
		})
	}

	tx, err := selectDatabase(userID).Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// 配布処理
	var (
		presentIDs []int64
		coinAmount int64
		cardIDs    []int64
		item45s    []obtain45Item
	)
	for i := range *obtainPresent {
		presentIDs = append(presentIDs, (*obtainPresent)[i].ID)
		(*obtainPresent)[i].UpdatedAt = requestAt
		(*obtainPresent)[i].DeletedAt = &requestAt
		switch (*obtainPresent)[i].ItemType {
		case 1: // coin
			coinAmount += int64((*obtainPresent)[i].Amount)
		case 2: // card
			cardIDs = append(cardIDs, (*obtainPresent)[i].ItemID)
		default:
			item45s = append(item45s, obtain45Item{
				itemID:       (*obtainPresent)[i].ItemID,
				obtainAmount: int64((*obtainPresent)[i].Amount),
			})
		}
	}

	if len(presentIDs) > 0 {
		q := "UPDATE user_presents SET deleted_at=?, updated_at=? WHERE id IN (?)"
		q, params, err = sqlx.In(q, requestAt, requestAt, presentIDs)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if _, err := tx.Exec(q, params...); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}

		if err := h.obtainCoin(tx, userID, coinAmount); err != nil {
			if err == ErrUserNotFound || err == ErrItemNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
			if err == ErrInvalidItemType {
				return errorResponse(c, http.StatusBadRequest, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err := h.obtainCards(tx, userID, requestAt, cardIDs); err != nil {
			if err == ErrUserNotFound || err == ErrItemNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
			if err == ErrInvalidItemType {
				return errorResponse(c, http.StatusBadRequest, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err := h.obtain45Items(tx, userID, requestAt, item45s); err != nil {
			if err == ErrUserNotFound || err == ErrItemNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
			if err == ErrInvalidItemType {
				return errorResponse(c, http.StatusBadRequest, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &ReceivePresentResponse{
		UpdatedResources: makeUpdatedResources(requestAt, nil, nil, nil, nil, nil, nil, *obtainPresent),
	})
}

type ReceivePresentRequest struct {
	ViewerID   string  `json:"viewerId"`
	PresentIDs []int64 `json:"presentIds"`
}

type ReceivePresentResponse struct {
	UpdatedResources *UpdatedResource `json:"updatedResources"`
}

// listItem アイテムリスト
// GET /user/{userID}/item
func (h *Handler) listItem(c *fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	user := userPool.get()
	defer userPool.put(user)
	query := "SELECT * FROM users WHERE id=?"
	if err = selectDatabase(userID).Get(user, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, ErrUserNotFound)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	itemList := userItemsArrPool.get()
	defer userItemsArrPool.put(itemList)
	query = "SELECT * FROM user_items WHERE user_id = ?"
	if err = selectDatabase(userID).Select(itemList, query, userID); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	cardList := userCardArrPool.get()
	defer userCardArrPool.put(cardList)
	query = "SELECT * FROM user_cards WHERE user_id=?"
	if err = selectDatabase(userID).Select(cardList, query, userID); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	tID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	tk, err := generateUUID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	token := &UserOneTimeToken{
		ID:        tID,
		UserID:    userID,
		Token:     tk,
		TokenType: 2,
		CreatedAt: requestAt,
		UpdatedAt: requestAt,
		ExpiredAt: requestAt + 600,
	}
	delete(oneTimeTokenType1, userID)
	oneTimeTokenType2[userID] = []*UserOneTimeToken{token}
	//go func() {
	//	// genearte one time token
	//	query := "UPDATE user_one_time_tokens SET deleted_at=? WHERE user_id=? AND deleted_at IS NULL"
	//	selectDatabase(userID).Exec(query, requestAt, userID)
	//	query = "INSERT INTO user_one_time_tokens(id, user_id, token, token_type, created_at, updated_at, expired_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
	//	selectDatabase(userID).Exec(query, token.ID, token.UserID, token.Token, token.TokenType, token.CreatedAt, token.UpdatedAt, token.ExpiredAt)
	//}()

	return successResponse(c, &ListItemResponse{
		OneTimeToken: token.Token,
		Items:        *itemList,
		User:         user,
		Cards:        *cardList,
	})
}

type ListItemResponse struct {
	OneTimeToken string      `json:"oneTimeToken"`
	User         *User       `json:"user"`
	Items        []*UserItem `json:"items"`
	Cards        []*UserCard `json:"cards"`
}

// addExpToCard 装備強化
// POST /user/{userID}/card/addexp/{cardID}
func (h *Handler) addExpToCard(c *fiber.Ctx) error {
	cardID, err := strconv.ParseInt(c.Params("cardID"), 10, 64)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	// read body
	req := addExpToCardRequestPool.get()
	defer addExpToCardRequestPool.put(req)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if err = h.checkOneTimeToken(userID, req.OneTimeToken, 2, requestAt); err != nil {
		if err == ErrInvalidToken {
			return errorResponse(c, http.StatusBadRequest, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	if err = h.checkViewerID(userID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// get target card
	card := targetUserCardDataPool.get()
	defer targetUserCardDataPool.put(card)
	query := `
	SELECT uc.id , uc.user_id , uc.card_id , uc.amount_per_sec , uc.level, uc.total_exp, im.amount_per_sec as 'base_amount_per_sec', im.max_level , im.max_amount_per_sec , im.base_exp_per_level
	FROM user_cards as uc
	INNER JOIN item_masters as im ON uc.card_id = im.id
	WHERE uc.id = ? AND uc.user_id=?
	`
	if err = selectDatabase(userID).Get(card, query, cardID, userID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	if card.Level == card.MaxLevel {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("target card is max level"))
	}

	// 消費アイテムの所持チェック
	items := make([]*ConsumeUserItemData, 0)
	query = `
	SELECT ui.id, ui.user_id, ui.item_id, ui.item_type, ui.amount, ui.created_at, ui.updated_at, im.gained_exp
	FROM user_items as ui
	INNER JOIN item_masters as im ON ui.item_id = im.id
	WHERE ui.item_type = 3 AND ui.id=? AND ui.user_id=?
	`
	for _, v := range req.Items {
		item := consumeUserItemDataPool.get()
		defer consumeUserItemDataPool.put(item)
		if err = selectDatabase(userID).Get(item, query, v.ID, userID); err != nil {
			if err == sql.ErrNoRows {
				return errorResponse(c, http.StatusNotFound, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}

		if v.Amount > item.Amount {
			return errorResponse(c, http.StatusBadRequest, fmt.Errorf("item not enough"))
		}
		item.ConsumeAmount = v.Amount
		items = append(items, item)
	}

	// 経験値付与
	// 経験値をカードに付与
	for _, v := range items {
		card.TotalExp += v.GainedExp * v.ConsumeAmount
	}

	// lvup判定(lv upしたら生産性を加算)
	for {
		nextLvThreshold := int(float64(card.BaseExpPerLevel) * math.Pow(1.2, float64(card.Level-1)))
		if nextLvThreshold > card.TotalExp {
			break
		}

		// lv up処理
		card.Level += 1
		card.AmountPerSec += (card.MaxAmountPerSec - card.BaseAmountPerSec) / (card.MaxLevel - 1)
	}

	tx, err := selectDatabase(userID).Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	defer tx.Rollback() //nolint:errcheck

	// cardのlvと経験値の更新、itemの消費
	query = "UPDATE user_cards SET amount_per_sec=?, level=?, total_exp=?, updated_at=? WHERE id=?"
	if _, err = tx.Exec(query, card.AmountPerSec, card.Level, card.TotalExp, requestAt, card.ID); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	query = "UPDATE user_items SET amount=?, updated_at=? WHERE id=?"
	for _, v := range items {
		if _, err = tx.Exec(query, v.Amount-v.ConsumeAmount, requestAt, v.ID); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	// get response data
	resultCard := userCardPool.get()
	defer userCardPool.put(resultCard)
	query = "SELECT * FROM user_cards WHERE id=?"
	if err = tx.Get(resultCard, query, card.ID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, fmt.Errorf("not found card"))
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	resultItems := userItemsArrPool.get()
	defer userItemsArrPool.put(resultItems)
	for _, v := range items {
		*resultItems = append(*resultItems, &UserItem{
			ID:        v.ID,
			UserID:    v.UserID,
			ItemID:    v.ItemID,
			ItemType:  v.ItemType,
			Amount:    v.Amount - v.ConsumeAmount,
			CreatedAt: v.CreatedAt,
			UpdatedAt: requestAt,
		})
	}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &AddExpToCardResponse{
		UpdatedResources: makeUpdatedResources(requestAt, nil, nil, []*UserCard{resultCard}, nil, *resultItems, nil, nil),
	})
}

type AddExpToCardRequest struct {
	ViewerID     string         `json:"viewerId"`
	OneTimeToken string         `json:"oneTimeToken"`
	Items        []*ConsumeItem `json:"items"`
}

type AddExpToCardResponse struct {
	UpdatedResources *UpdatedResource `json:"updatedResources"`
}

type ConsumeItem struct {
	ID     int64 `json:"id"`
	Amount int   `json:"amount"`
}

type ConsumeUserItemData struct {
	ID        int64 `db:"id"`
	UserID    int64 `db:"user_id"`
	ItemID    int64 `db:"item_id"`
	ItemType  int   `db:"item_type"`
	Amount    int   `db:"amount"`
	CreatedAt int64 `db:"created_at"`
	UpdatedAt int64 `db:"updated_at"`
	GainedExp int   `db:"gained_exp"`

	ConsumeAmount int // 消費量
}

type TargetUserCardData struct {
	ID           int64 `db:"id"`
	UserID       int64 `db:"user_id"`
	CardID       int64 `db:"card_id"`
	AmountPerSec int   `db:"amount_per_sec"`
	Level        int   `db:"level"`
	TotalExp     int   `db:"total_exp"`

	// lv1のときの生産性
	BaseAmountPerSec int `db:"base_amount_per_sec"`
	// 最高レベル
	MaxLevel int `db:"max_level"`
	// lv maxのときの生産性
	MaxAmountPerSec int `db:"max_amount_per_sec"`
	// lv1 -> lv2に上がるときのexp
	BaseExpPerLevel int `db:"base_exp_per_level"`
}

// updateDeck 装備変更
// POST /user/{userID}/card
func (h *Handler) updateDeck(c *fiber.Ctx) error {

	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	// read body
	req := updateDeckRequestPool.get()
	defer updateDeckRequestPool.put(req)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	if len(req.CardIDs) != DeckCardNumber {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid number of cards"))
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if err = h.checkViewerID(userID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// カード所持情報のバリデーション
	query := "SELECT * FROM user_cards WHERE id IN (?)"
	query, params, err := sqlx.In(query, req.CardIDs)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}
	cards := userCardArrPool.get()
	defer userCardArrPool.put(cards)
	if err = selectDatabase(userID).Select(cards, query, params...); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	if len(*cards) != DeckCardNumber {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid card ids"))
	}

	tx, err := selectDatabase(userID).Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	defer tx.Rollback() //nolint:errcheck

	// update data
	query = "UPDATE user_decks SET updated_at=?, deleted_at=? WHERE user_id=? AND deleted_at IS NULL"
	if _, err = tx.Exec(query, requestAt, requestAt, userID); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	udID, err := h.generateID()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	newDeck := &UserDeck{
		ID:        udID,
		UserID:    userID,
		CardID1:   req.CardIDs[0],
		CardID2:   req.CardIDs[1],
		CardID3:   req.CardIDs[2],
		CreatedAt: requestAt,
		UpdatedAt: requestAt,
	}
	query = "INSERT INTO user_decks(id, user_id, user_card_id_1, user_card_id_2, user_card_id_3, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
	if _, err := tx.Exec(query, newDeck.ID, newDeck.UserID, newDeck.CardID1, newDeck.CardID2, newDeck.CardID3, newDeck.CreatedAt, newDeck.UpdatedAt); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &UpdateDeckResponse{
		UpdatedResources: makeUpdatedResources(requestAt, nil, nil, nil, []*UserDeck{newDeck}, nil, nil, nil),
	})
}

type UpdateDeckRequest struct {
	ViewerID string  `json:"viewerId"`
	CardIDs  []int64 `json:"cardIds"`
}

type UpdateDeckResponse struct {
	UpdatedResources *UpdatedResource `json:"updatedResources"`
}

// reward ゲーム報酬受取
// POST /user/{userID}/reward
func (h *Handler) reward(c *fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	// parse body
	req := rewardRequestPool.get()
	defer rewardRequestPool.put(req)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if err = h.checkViewerID(userID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// 最後に取得した報酬時刻取得
	user := userPool.get()
	defer userPool.put(user)
	query := "SELECT * FROM users WHERE id=?"
	if err = selectDatabase(userID).Get(user, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, ErrUserNotFound)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// 使っているデッキの取得
	deck := userDeckPool.get()
	defer userDeckPool.put(deck)
	query = "SELECT * FROM user_decks WHERE user_id=? AND deleted_at IS NULL"
	if err = selectDatabase(userID).Get(deck, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	cards := userCardArrPool.get()
	defer userCardArrPool.put(cards)
	query = "SELECT * FROM user_cards WHERE id IN (?, ?, ?)"
	if err = selectDatabase(userID).Select(cards, query, deck.CardID1, deck.CardID2, deck.CardID3); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	if len(*cards) != 3 {
		return errorResponse(c, http.StatusBadRequest, fmt.Errorf("invalid cards length"))
	}

	// 経過時間*生産性のcoin (1椅子 = 1coin)
	pastTime := requestAt - user.LastGetRewardAt
	getCoin := int(pastTime) * ((*cards)[0].AmountPerSec + (*cards)[1].AmountPerSec + (*cards)[2].AmountPerSec)

	// 報酬の保存(ゲームない通貨を保存)(users)
	user.IsuCoin += int64(getCoin)
	user.LastGetRewardAt = requestAt

	query = "UPDATE users SET isu_coin=?, last_getreward_at=? WHERE id=?"
	if _, err = selectDatabase(userID).Exec(query, user.IsuCoin, user.LastGetRewardAt, user.ID); err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &RewardResponse{
		UpdatedResources: makeUpdatedResources(requestAt, user, nil, nil, nil, nil, nil, nil),
	})
}

type RewardRequest struct {
	ViewerID string `json:"viewerId"`
}

type RewardResponse struct {
	UpdatedResources *UpdatedResource `json:"updatedResources"`
}

// home ホーム取得
// GET /user/{userID}/home
func (h *Handler) home(c *fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	// 装備情報
	deck := userDeckPool.get()
	defer userDeckPool.put(deck)
	query := "SELECT * FROM user_decks WHERE user_id=? AND deleted_at IS NULL"
	if err = selectDatabase(userID).Get(deck, query, userID); err != nil {
		if err != sql.ErrNoRows {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		deck = nil
	}

	// 生産性
	cards := userCardArrPool.get()
	defer userCardArrPool.put(cards)
	if deck != nil {
		cardIds := []int64{deck.CardID1, deck.CardID2, deck.CardID3}
		query, params, err := sqlx.In("SELECT * FROM user_cards WHERE id IN (?)", cardIds)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err = selectDatabase(userID).Select(cards, query, params...); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}
	totalAmountPerSec := 0
	for _, v := range *cards {
		totalAmountPerSec += v.AmountPerSec
	}

	// 経過時間
	user := userPool.get()
	defer userPool.put(user)
	query = "SELECT * FROM users WHERE id=?"
	if err = selectDatabase(userID).Get(user, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return errorResponse(c, http.StatusNotFound, ErrUserNotFound)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	pastTime := requestAt - user.LastGetRewardAt

	return successResponse(c, &HomeResponse{
		Now:               requestAt,
		User:              user,
		Deck:              deck,
		TotalAmountPerSec: totalAmountPerSec,
		PastTime:          pastTime,
	})
}

type HomeResponse struct {
	Now               int64     `json:"now"`
	User              *User     `json:"user"`
	Deck              *UserDeck `json:"deck,omitempty"`
	TotalAmountPerSec int       `json:"totalAmountPerSec"`
	PastTime          int64     `json:"pastTime"` // 経過時間を秒単位で
}

// //////////////////////////////////////
// util

// health ヘルスチェック
func (h *Handler) health(c *fiber.Ctx) error {
	return c.Status(http.StatusOK).SendString("OK")
}

// errorResponse returns error.
func errorResponse(c *fiber.Ctx, statusCode int, err error) error {
	// log.Printf("status=%d, err=%+v", statusCode, errors.WithStack(err))

	return c.Status(statusCode).JSON(struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
	}{
		StatusCode: statusCode,
		Message:    err.Error(),
	})
}

// successResponse responds success.
func successResponse(c *fiber.Ctx, v interface{}) error {
	return c.Status(http.StatusOK).JSON(v)
}

// noContentResponse
func noContentResponse(c *fiber.Ctx, status int) error {
	return c.SendStatus(status)
}

// generateID uniqueなIDを生成する
func (h *Handler) generateID() (int64, error) {
	return h.snowflakeNode.Generate().Int64(), nil
}

var (
	uuidCh = make(chan string, 10000)
)

// generateSessionID
func generateUUID() (string, error) {
	return xid.New().String(), nil
}

// getUserID gets userID by path param.
func getUserID(c *fiber.Ctx) (int64, error) {
	return strconv.ParseInt(c.Params("userID"), 10, 64)
}

// getEnv gets environment variable.
func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v == "" {
		return defaultVal
	} else {
		return v
	}
}

// parseRequestBody parses request body.
func parseRequestBody(c *fiber.Ctx, dist interface{}) error {
	err := c.BodyParser(dist)
	if err != nil {
		return ErrInvalidRequestBody
	}
	return nil
}

type UpdatedResource struct {
	Now  int64 `json:"now"`
	User *User `json:"user,omitempty"`

	UserDevice       *UserDevice       `json:"userDevice,omitempty"`
	UserCards        []*UserCard       `json:"userCards,omitempty"`
	UserDecks        []*UserDeck       `json:"userDecks,omitempty"`
	UserItems        []*UserItem       `json:"userItems,omitempty"`
	UserLoginBonuses []*UserLoginBonus `json:"userLoginBonuses,omitempty"`
	UserPresents     []*UserPresent    `json:"userPresents,omitempty"`
}

func makeUpdatedResources(
	requestAt int64,
	user *User,
	userDevice *UserDevice,
	userCards []*UserCard,
	userDecks []*UserDeck,
	userItems []*UserItem,
	userLoginBonuses []*UserLoginBonus,
	userPresents []*UserPresent,
) *UpdatedResource {
	return &UpdatedResource{
		Now:              requestAt,
		User:             user,
		UserDevice:       userDevice,
		UserCards:        userCards,
		UserItems:        userItems,
		UserDecks:        userDecks,
		UserLoginBonuses: userLoginBonuses,
		UserPresents:     userPresents,
	}
}

// //////////////////////////////////////
// entity

type User struct {
	ID              int64  `json:"id" db:"id"`
	IsuCoin         int64  `json:"isuCoin" db:"isu_coin"`
	LastGetRewardAt int64  `json:"lastGetRewardAt" db:"last_getreward_at"`
	LastActivatedAt int64  `json:"lastActivatedAt" db:"last_activated_at"`
	RegisteredAt    int64  `json:"registeredAt" db:"registered_at"`
	CreatedAt       int64  `json:"createdAt" db:"created_at"`
	UpdatedAt       int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt       *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserDevice struct {
	ID           int64  `json:"id" db:"id"`
	UserID       int64  `json:"userId" db:"user_id"`
	PlatformID   string `json:"platformId" db:"platform_id"`
	PlatformType int    `json:"platformType" db:"platform_type"`
	CreatedAt    int64  `json:"createdAt" db:"created_at"`
	UpdatedAt    int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt    *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserBan struct {
	ID        int64  `db:"id"`
	UserID    int64  `db:"user_id"`
	CreatedAt int64  `db:"created_at"`
	UpdatedAt int64  `db:"updated_at"`
	DeletedAt *int64 `db:"deleted_at"`
}

type UserCard struct {
	ID           int64  `json:"id" db:"id"`
	UserID       int64  `json:"userId" db:"user_id"`
	CardID       int64  `json:"cardId" db:"card_id"`
	AmountPerSec int    `json:"amountPerSec" db:"amount_per_sec"`
	Level        int    `json:"level" db:"level"`
	TotalExp     int64  `json:"totalExp" db:"total_exp"`
	CreatedAt    int64  `json:"createdAt" db:"created_at"`
	UpdatedAt    int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt    *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserDeck struct {
	ID        int64  `json:"id" db:"id"`
	UserID    int64  `json:"userId" db:"user_id"`
	CardID1   int64  `json:"cardId1" db:"user_card_id_1"`
	CardID2   int64  `json:"cardId2" db:"user_card_id_2"`
	CardID3   int64  `json:"cardId3" db:"user_card_id_3"`
	CreatedAt int64  `json:"createdAt" db:"created_at"`
	UpdatedAt int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserItem struct {
	ID        int64  `json:"id" db:"id"`
	UserID    int64  `json:"userId" db:"user_id"`
	ItemType  int    `json:"itemType" db:"item_type"`
	ItemID    int64  `json:"itemId" db:"item_id"`
	Amount    int    `json:"amount" db:"amount"`
	CreatedAt int64  `json:"createdAt" db:"created_at"`
	UpdatedAt int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserLoginBonus struct {
	ID                 int64  `json:"id" db:"id"`
	UserID             int64  `json:"userId" db:"user_id"`
	LoginBonusID       int64  `json:"loginBonusId" db:"login_bonus_id"`
	LastRewardSequence int    `json:"lastRewardSequence" db:"last_reward_sequence"`
	LoopCount          int    `json:"loopCount" db:"loop_count"`
	CreatedAt          int64  `json:"createdAt" db:"created_at"`
	UpdatedAt          int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt          *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserPresent struct {
	ID             int64  `json:"id" db:"id"`
	UserID         int64  `json:"userId" db:"user_id"`
	SentAt         int64  `json:"sentAt" db:"sent_at"`
	ItemType       int    `json:"itemType" db:"item_type"`
	ItemID         int64  `json:"itemId" db:"item_id"`
	Amount         int    `json:"amount" db:"amount"`
	PresentMessage string `json:"presentMessage" db:"present_message"`
	CreatedAt      int64  `json:"createdAt" db:"created_at"`
	UpdatedAt      int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt      *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserPresentAllReceivedHistory struct {
	ID           int64  `json:"id" db:"id"`
	UserID       int64  `json:"userId" db:"user_id"`
	PresentAllID int64  `json:"presentAllId" db:"present_all_id"`
	ReceivedAt   int64  `json:"receivedAt" db:"received_at"`
	CreatedAt    int64  `json:"createdAt" db:"created_at"`
	UpdatedAt    int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt    *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type Session struct {
	ID        int64  `json:"id" db:"id"`
	UserID    int64  `json:"userId" db:"user_id"`
	SessionID string `json:"sessionId" db:"session_id"`
	ExpiredAt int64  `json:"expiredAt" db:"expired_at"`
	CreatedAt int64  `json:"createdAt" db:"created_at"`
	UpdatedAt int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

type UserOneTimeToken struct {
	ID        int64  `json:"id" db:"id"`
	UserID    int64  `json:"userId" db:"user_id"`
	Token     string `json:"token" db:"token"`
	TokenType int    `json:"tokenType" db:"token_type"`
	ExpiredAt int64  `json:"expiredAt" db:"expired_at"`
	CreatedAt int64  `json:"createdAt" db:"created_at"`
	UpdatedAt int64  `json:"updatedAt" db:"updated_at"`
	DeletedAt *int64 `json:"deletedAt,omitempty" db:"deleted_at"`
}

// //////////////////////////////////////
// master

type GachaMaster struct {
	ID           int64  `json:"id" db:"id"`
	Name         string `json:"name" db:"name"`
	StartAt      int64  `json:"startAt" db:"start_at"`
	EndAt        int64  `json:"endAt" db:"end_at"`
	DisplayOrder int    `json:"displayOrder" db:"display_order"`
	CreatedAt    int64  `json:"createdAt" db:"created_at"`
}

type GachaItemMaster struct {
	ID        int64 `json:"id" db:"id"`
	GachaID   int64 `json:"gachaId" db:"gacha_id"`
	ItemType  int   `json:"itemType" db:"item_type"`
	ItemID    int64 `json:"itemId" db:"item_id"`
	Amount    int   `json:"amount" db:"amount"`
	Weight    int   `json:"weight" db:"weight"`
	CreatedAt int64 `json:"createdAt" db:"created_at"`
}

type ItemMaster struct {
	ID              int64  `json:"id" db:"id"`
	ItemType        int    `json:"itemType" db:"item_type"`
	Name            string `json:"name" db:"name"`
	Description     string `json:"description" db:"description"`
	AmountPerSec    *int   `json:"amountPerSec" db:"amount_per_sec"`
	MaxLevel        *int   `json:"maxLevel" db:"max_level"`
	MaxAmountPerSec *int   `json:"maxAmountPerSec" db:"max_amount_per_sec"`
	BaseExpPerLevel *int   `json:"baseExpPerLevel" db:"base_exp_per_level"`
	GainedExp       *int   `json:"gainedExp" db:"gained_exp"`
	ShorteningMin   *int64 `json:"shorteningMin" db:"shortening_min"`
	// CreatedAt       int64 `json:"createdAt"`
}

type LoginBonusMaster struct {
	ID          int64 `json:"id" db:"id"`
	StartAt     int64 `json:"startAt" db:"start_at"`
	EndAt       int64 `json:"endAt" db:"end_at"`
	ColumnCount int   `json:"columnCount" db:"column_count"`
	Looped      bool  `json:"looped" db:"looped"`
	CreatedAt   int64 `json:"createdAt" db:"created_at"`
}

type LoginBonusRewardMaster struct {
	ID             int64 `json:"id" db:"id"`
	LoginBonusID   int64 `json:"loginBonusId" db:"login_bonus_id"`
	RewardSequence int   `json:"rewardSequence" db:"reward_sequence"`
	ItemType       int   `json:"itemType" db:"item_type"`
	ItemID         int64 `json:"itemId" db:"item_id"`
	Amount         int64 `json:"amount" db:"amount"`
	CreatedAt      int64 `json:"createdAt" db:"created_at"`
}

type PresentAllMaster struct {
	ID                int64  `json:"id" db:"id"`
	RegisteredStartAt int64  `json:"registeredStartAt" db:"registered_start_at"`
	RegisteredEndAt   int64  `json:"registeredEndAt" db:"registered_end_at"`
	ItemType          int    `json:"itemType" db:"item_type"`
	ItemID            int64  `json:"itemId" db:"item_id"`
	Amount            int64  `json:"amount" db:"amount"`
	PresentMessage    string `json:"presentMessage" db:"present_message"`
	CreatedAt         int64  `json:"createdAt" db:"created_at"`
}

type VersionMaster struct {
	ID            int64  `json:"id" db:"id"`
	Status        int    `json:"status" db:"status"`
	MasterVersion string `json:"masterVersion" db:"master_version"`
}
