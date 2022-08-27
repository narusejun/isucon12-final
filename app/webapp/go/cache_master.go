package main

import (
	"sync"

	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/singleflight"
)

var (
	masterVersion = "nothing"
	sf            = &singleflight.Group{}

	itemMasterMux             = &sync.RWMutex{}
	itemMaster                = make([]*ItemMaster, 0)
	gachaMasterMux            = &sync.RWMutex{}
	gachaMaster               = make([]*GachaMaster, 0)
	gachaItemMasterMux        = &sync.RWMutex{}
	gachaItemMaster           = make([]*GachaItemMaster, 0)
	presentAllMasterMux       = &sync.RWMutex{}
	presentAllMaster          = make([]*PresentAllMaster, 0)
	loginBonusMasterMux       = &sync.RWMutex{}
	loginBonusMaster          = make([]*LoginBonusMaster, 0)
	loginBonusRewardMasterMux = &sync.RWMutex{}
	loginBonusRewardMaster    = make([]*LoginBonusRewardMaster, 0)
)

func shouldRecache(db *sqlx.DB) (string, error) {
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

func recache(db *sqlx.DB) error {
	itemMasterMux.Lock()
	defer itemMasterMux.Unlock()
	gachaMasterMux.Lock()
	defer gachaMasterMux.Unlock()
	gachaItemMasterMux.Lock()
	defer gachaItemMasterMux.Unlock()
	presentAllMasterMux.Lock()
	defer presentAllMasterMux.Unlock()
	loginBonusMasterMux.Lock()
	defer loginBonusMasterMux.Unlock()
	loginBonusRewardMasterMux.Lock()
	defer loginBonusRewardMasterMux.Unlock()

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
	itemMasterMux.RLock()
	defer itemMasterMux.RUnlock()
	for _, v := range itemMaster {
		if v.ID == id {
			return true, *v
		}
	}

	return false, ItemMaster{}
}

func getGachaMaster(requestAt int64) []*GachaMaster {
	gachaMasterMux.RLock()
	defer gachaMasterMux.RUnlock()
	masters := make([]*GachaMaster, 0)
	for _, v := range gachaMaster {
		if v.StartAt <= requestAt && requestAt <= v.EndAt {
			masters = append(masters, v)
		}
	}

	return masters
}

func getGachaMasterByID(id int64, requestAt int64) (bool, GachaMaster) {
	gachaMasterMux.RLock()
	defer gachaMasterMux.RUnlock()
	for _, v := range gachaMaster {
		if v.ID == id && v.StartAt <= requestAt && requestAt <= v.EndAt {
			return true, *v
		}
	}

	return false, GachaMaster{}
}

func getGachaItemMasterByID(id int64) (bool, []*GachaItemMaster) {
	gachaItemMasterMux.RLock()
	defer gachaItemMasterMux.RUnlock()
	gachaItems := make([]*GachaItemMaster, 0)
	for _, v := range gachaItemMaster {
		if v.GachaID == id {
			gachaItems = append(gachaItems, v)
		}
	}

	return len(gachaItems) != 0, gachaItems
}

func getPresentAllMaster(requestAt int64) []*PresentAllMaster {
	presentAllMasterMux.RLock()
	defer presentAllMasterMux.RUnlock()
	masters := make([]*PresentAllMaster, 0)
	for _, v := range presentAllMaster {
		if v.RegisteredStartAt <= requestAt && requestAt <= v.RegisteredEndAt {
			masters = append(masters, v)
		}
	}

	return masters
}

func getLoginBonusMaster(requestAt int64) []*LoginBonusMaster {
	loginBonusMasterMux.RLock()
	defer loginBonusMasterMux.RUnlock()
	masters := make([]*LoginBonusMaster, 0)
	for _, v := range loginBonusMaster {
		if v.StartAt <= requestAt && requestAt <= v.EndAt {
			masters = append(masters, v)
		}
	}

	return masters
}

func getLoginBonusRewardMasterByIDAndSequence(loginBonusID int64, rewardSequence int) (bool, LoginBonusRewardMaster) {
	loginBonusRewardMasterMux.RLock()
	defer loginBonusRewardMasterMux.RUnlock()
	for _, v := range loginBonusRewardMaster {
		if v.LoginBonusID == loginBonusID && v.RewardSequence == rewardSequence {
			return true, *v
		}
	}
	return false, LoginBonusRewardMaster{}
}
