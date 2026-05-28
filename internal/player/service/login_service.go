package service

type LoginResult struct {
	PlayerID int64
	WorldID  int64
	Account  string
}

type LoginService interface {
	Login(playerID int64, worldID int64, account string) (LoginResult, error)
}

type StaticLoginService struct{}

func NewStaticLoginService() StaticLoginService {
	return StaticLoginService{}
}

func (StaticLoginService) Login(playerID int64, worldID int64, account string) (LoginResult, error) {
	return LoginResult{
		PlayerID: playerID,
		WorldID:  worldID,
		Account:  account,
	}, nil
}
