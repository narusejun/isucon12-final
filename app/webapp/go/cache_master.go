package main

import (
	"sync"

	"github.com/jmoiron/sqlx"
)

var (
	masterVersion = "nothing"
	masterMux     = &sync.RWMutex{}

	loginBonusMaster = make([]*LoginBonusMaster, 0)
	itemMaster       = make([]*ItemMaster, 0)
	gachaMaster      = make([]*GachaMaster, 0)
	gachaItemMaster  = make([]*GachaItemMaster, 0)
)

func shouldRecache(db *sqlx.DB) (string, error) {
	tmp := ""
	db.Get(&tmp, "SELECT master_version FROM version_masters WHERE status = 1")
	if tmp != masterVersion {
		if err := recache(db); err != nil {
			return masterVersion, err
		}
		masterVersion = tmp
		return masterVersion, nil
	}
	return masterVersion, nil
}

func recache(db *sqlx.DB) error {
	masterMux.Lock()
	defer masterMux.Unlock()
	query := "SELECT * FROM login_bonus_masters"
	if err := db.Select(&loginBonusMaster, query); err != nil {
		return err
	}

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

	return nil
}

func getLoginBonusMaster(requestAt int64) []*LoginBonusMaster {
	masterMux.RLock()
	defer masterMux.RUnlock()
	masters := make([]*LoginBonusMaster, 0)
	for _, v := range loginBonusMaster {
		if v.StartAt <= requestAt && requestAt <= v.EndAt {
			masters = append(masters, v)
		}
	}

	return masters
}

func getItemMasterByID(id int64) (bool, ItemMaster) {
	masterMux.RLock()
	defer masterMux.RUnlock()
	for _, v := range itemMaster {
		if v.ID == id {
			return true, *v
		}
	}

	return false, ItemMaster{}
}

func getGachaMaster(requestAt int64) []*GachaMaster {
	masterMux.RLock()
	defer masterMux.RUnlock()
	masters := make([]*GachaMaster, 0)
	for _, v := range gachaMaster {
		if v.StartAt <= requestAt && requestAt <= v.EndAt {
			masters = append(masters, v)
		}
	}

	return masters
}

func getGachaMasterByID(id int64, requestAt int64) (bool, GachaMaster) {
	masterMux.RLock()
	defer masterMux.RUnlock()
	for _, v := range gachaMaster {
		if v.ID == id && v.StartAt <= requestAt && requestAt <= v.EndAt {
			return true, *v
		}
	}

	return false, GachaMaster{}
}

func getGachaItemMasterByID(id int64) (bool, []*GachaItemMaster) {
	masterMux.RLock()
	defer masterMux.RUnlock()
	gachaItems := make([]*GachaItemMaster, 0)
	for _, v := range gachaItemMaster {
		if v.GachaID == id {
			gachaItems = append(gachaItems, v)
		}
	}

	return len(gachaItems) != 0, gachaItems
}
