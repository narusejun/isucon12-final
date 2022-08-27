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
