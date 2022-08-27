package main

import "sync"

var (
	int64ArrPool                         = newArrPool[int64](0)
	userLoginBonusArrPool                = newArrPool[*UserLoginBonus](0)
	userItemsArrPool                     = newArrPool[*UserItem](0)
	userPresentArrPool                   = newArrPool[*UserPresent](0)
	loginBonusRewardMasterArrPool        = newArrPool[*LoginBonusRewardMaster](0)
	userPresentAllReceivedHistoryArrPool = newArrPool[*UserPresentAllReceivedHistory](0)
	itemMasterArrPool                    = newArrPool[ItemMaster](0)
)

type arrPool[T any] struct {
	data *sync.Pool
}

func newArrPool[T any](defaultSize int) *arrPool[T] {
	return &arrPool[T]{
		data: &sync.Pool{
			New: func() interface{} {
				s := make([]T, 0, defaultSize)
				return &s
			},
		},
	}
}

func (p *arrPool[T]) get() ([]T, func()) {
	ptr := p.data.Get().(*[]T)
	arr := *ptr
	return arr, func() {
		arr = arr[0:0]
		*ptr = arr
		p.data.Put(ptr)
	}
}

type pool[T any] struct {
	data *sync.Pool
	putF func(T) T
}

func newPool[T any](fn func() T, putF func(T) T) *pool[T] {
	return &pool[T]{
		data: &sync.Pool{
			New: func() interface{} {
				return fn()
			},
		},
		putF: putF,
	}
}

func (p *pool[T]) get() (T, func()) {
	res := p.data.Get().(T)
	return res, func() {
		p.data.Put(p.putF(res))
	}
}
