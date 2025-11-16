package domain

type Member struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

func NewMember(token string, user *User) *Member {
	return &Member{
		Token: token,
		User:  user,
	}
}
