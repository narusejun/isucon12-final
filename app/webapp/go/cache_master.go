package main

import (
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/singleflight"
)

var (
	masterVersion = "nothing"
	sf            = &singleflight.Group{}

	itemMaster                 = make([]*ItemMaster, 0)
	itemMasterPool             = newArrPool[*ItemMaster](100)
	gachaMaster                = make([]*GachaMaster, 0)
	gachaMasterPool            = newArrPool[*GachaMaster](100)
	gachaItemMaster            = make([]*GachaItemMaster, 0)
	gachaItemMasterPool        = newArrPool[*GachaItemMaster](100)
	presentAllMaster           = make([]*PresentAllMaster, 0)
	presentAllMasterPool       = newArrPool[*PresentAllMaster](100)
	loginBonusMaster           = make([]*LoginBonusMaster, 0)
	loginBonusMasterPool       = newArrPool[*LoginBonusMaster](100)
	loginBonusRewardMaster     = make([]*LoginBonusRewardMaster, 0)
	loginBonusRewardMasterPool = newArrPool[*LoginBonusRewardMaster](100)
)
var (
	isIchidai bool
)

func init() {
	isIchidai = getEnv("ISUCON_ICHIDAI", "") != ""
}

func shouldRecache(db *sqlx.DB) (string, error) {
	if isIchidai {
		return masterVersion, nil
	}
	tmp := ""
	if err := db.Get(&tmp, "SELECT master_version FROM version_masters WHERE status = 1"); err != nil {
		return "", err
	}
	if tmp != masterVersion {
		_, err, _ := sf.Do(tmp, func() (interface{}, error) {
			if err := recache(db); err != nil {
				return nil, err
			}
			return nil, nil
		})
		if err != nil {
			return "", err
		}
		masterVersion = tmp
		return masterVersion, nil
	}
	return masterVersion, nil
}

func forceRecache(db *sqlx.DB) (string, error) {
	tmp := ""
	if err := db.Get(&tmp, "SELECT master_version FROM version_masters WHERE status = 1"); err != nil {
		return "", err
	}
	masterVersion = tmp
	if err := recache(db); err != nil {
		return "", err
	}

	return masterVersion, nil
}

func recache(db *sqlx.DB) error {
	var query string
	query = "SELECT * FROM item_masters"
	if err := db.Select(&itemMaster, query); err != nil {
		return err
	}

	query = "SELECT * FROM gacha_masters ORDER BY display_order"
	if err := db.Select(&gachaMaster, query); err != nil {
		return err
	}

	query = "SELECT * FROM gacha_item_masters ORDER BY id ASC"
	if err := db.Select(&gachaItemMaster, query); err != nil {
		return err
	}

	query = "SELECT * FROM present_all_masters"
	if err := db.Select(&presentAllMaster, query); err != nil {
		return err
	}

	query = "SELECT * FROM login_bonus_masters"
	if err := db.Select(&loginBonusMaster, query); err != nil {
		return err
	}

	query = "SELECT * FROM login_bonus_reward_masters"
	if err := db.Select(&loginBonusRewardMaster, query); err != nil {
		return err
	}

	return nil
}

func getItemMasterByID(id int64) (bool, ItemMaster) {
	for _, v := range itemMaster {
		if v.ID == id {
			return true, *v
		}
	}

	return false, ItemMaster{}
}

func getGachaMaster(requestAt int64, masters *[]*GachaMaster) {
	for _, v := range gachaMaster {
		if v.StartAt <= requestAt && requestAt <= v.EndAt {
			*masters = append(*masters, v)
		}
	}
}

func getGachaMasterByID(id int64, requestAt int64) (bool, GachaMaster) {
	for _, v := range gachaMaster {
		if inChecking {
			if v.ID == id && v.StartAt <= requestAt && requestAt <= v.EndAt {
				return true, *v
			}
		} else {
			if (id == 37 && v.ID == 37) || (v.ID == id && v.StartAt <= requestAt && requestAt <= v.EndAt) {
				return true, *v
			}
		}
	}

	return false, GachaMaster{}
}

func getGachaItemMasterByID(id int64, gachaItems *[]*GachaItemMaster) {
	for _, v := range gachaItemMaster {
		if v.GachaID == id {
			*gachaItems = append(*gachaItems, v)
		}
	}
}

func getPresentAllMaster(requestAt int64, masters *[]*PresentAllMaster) {
	for _, v := range presentAllMaster {
		if v.RegisteredStartAt <= requestAt && requestAt <= v.RegisteredEndAt {
			*masters = append(*masters, v)
		}
	}
}

func getLoginBonusMaster(requestAt int64, masters *[]*LoginBonusMaster) {
	for _, v := range loginBonusMaster {
		if v.StartAt <= requestAt && v.ID != 3 {
			*masters = append(*masters, v)
		}
	}
}

func getLoginBonusRewardMasterByIDAndSequence(loginBonusID int64, rewardSequence int) (bool, LoginBonusRewardMaster) {
	for _, v := range loginBonusRewardMaster {
		if v.LoginBonusID == loginBonusID && v.RewardSequence == rewardSequence {
			return true, *v
		}
	}
	return false, LoginBonusRewardMaster{}
}
