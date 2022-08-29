package main

import "sync"

var (
	int64ArrPool                         = newArrPool[int64](100)
	stringArrPool                        = newArrPool[string](100)
	userLoginBonusArrPool                = newArrPool[*UserLoginBonus](100)
	userItemsArrPool                     = newArrPool[*UserItem](100)
	userPresentArrPool                   = newArrPool[*UserPresent](100)
	loginBonusRewardMasterArrPool        = newArrPool[*LoginBonusRewardMaster](100)
	userPresentAllReceivedHistoryArrPool = newArrPool[*UserPresentAllReceivedHistory](100)
	itemMasterArrPool                    = newArrPool[ItemMaster](100)
	userCardArrPool                      = newArrPool[*UserCard](100)

	sessionPool = newPool[Session](func() *Session {
		return &Session{}
	}, func(s *Session) *Session {
		return s
	})

	userOneTimeTokenPool = newPool[UserOneTimeToken](func() *UserOneTimeToken {
		return &UserOneTimeToken{}
	}, func(s *UserOneTimeToken) *UserOneTimeToken {
		return s
	})

	userDevicePool = newPool[UserDevice](func() *UserDevice {
		return &UserDevice{}
	}, func(s *UserDevice) *UserDevice {
		return s
	})

	userPool = newPool[User](func() *User {
		return &User{}
	}, func(s *User) *User {
		s.ID = 0
		s.IsuCoin = 0
		s.LastActivatedAt = 0
		s.LastActivatedAt = 0
		s.RegisteredAt = 0
		s.CreatedAt = 0
		s.UpdatedAt = 0
		s.DeletedAt = nil
		return s
	})

	createUserRequestPool = newPool[CreateUserRequest](func() *CreateUserRequest {
		return &CreateUserRequest{}
	}, func(s *CreateUserRequest) *CreateUserRequest {
		return s
	})

	loginRequestPool = newPool[LoginRequest](func() *LoginRequest {
		return &LoginRequest{}
	}, func(s *LoginRequest) *LoginRequest {
		return s
	})

	drawGachaRequestPool = newPool[DrawGachaRequest](func() *DrawGachaRequest {
		return &DrawGachaRequest{}
	}, func(s *DrawGachaRequest) *DrawGachaRequest {
		return s
	})

	receivePresentRequestPool = newPool[ReceivePresentRequest](func() *ReceivePresentRequest {
		return &ReceivePresentRequest{}
	}, func(s *ReceivePresentRequest) *ReceivePresentRequest {
		return s
	})

	addExpToCardRequestPool = newPool[AddExpToCardRequest](func() *AddExpToCardRequest {
		return &AddExpToCardRequest{}
	}, func(s *AddExpToCardRequest) *AddExpToCardRequest {
		return s
	})

	targetUserCardDataPool = newPool[TargetUserCardData](func() *TargetUserCardData {
		return &TargetUserCardData{}
	}, func(s *TargetUserCardData) *TargetUserCardData {
		return s
	})

	consumeUserItemDataPool = newPool[ConsumeUserItemData](func() *ConsumeUserItemData {
		return &ConsumeUserItemData{}
	}, func(s *ConsumeUserItemData) *ConsumeUserItemData {
		return s
	})

	userCardPool = newPool[UserCard](func() *UserCard {
		return &UserCard{}
	}, func(s *UserCard) *UserCard {
		return s
	})

	updateDeckRequestPool = newPool[UpdateDeckRequest](func() *UpdateDeckRequest {
		return &UpdateDeckRequest{}
	}, func(s *UpdateDeckRequest) *UpdateDeckRequest {
		return s
	})

	rewardRequestPool = newPool[RewardRequest](func() *RewardRequest {
		return &RewardRequest{}
	}, func(s *RewardRequest) *RewardRequest {
		return s
	})

	userDeckPool = newPool[UserDeck](func() *UserDeck {
		return &UserDeck{}
	}, func(s *UserDeck) *UserDeck {
		return s
	})
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

func (p *arrPool[T]) get() *[]T {
	return p.data.Get().(*[]T)
}

func (p *arrPool[T]) put(ptr *[]T) {
	(*ptr) = (*ptr)[:0]
	p.data.Put(ptr)
}

type pool[T any] struct {
	data *sync.Pool
	putF func(*T) *T
}

func newPool[T any](fn func() *T, putF func(*T) *T) *pool[T] {
	return &pool[T]{
		data: &sync.Pool{
			New: func() interface{} {
				return fn()
			},
		},
		putF: putF,
	}
}

func (p *pool[T]) get() *T {
	res := p.data.Get().(*T)
	return res
}

func (p *pool[T]) put(res *T) {
	p.data.Put(p.putF(res))
}
