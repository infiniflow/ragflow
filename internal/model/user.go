package model

import "time"

// User user model
type User struct {
	ID              string     `gorm:"column:id;size:32;primaryKey" json:"id"`
	AccessToken     *string    `gorm:"column:access_token;size:255;index" json:"access_token,omitempty"`
	Nickname        string     `gorm:"column:nickname;size:100;not null;index" json:"nickname"`
	Password        *string    `gorm:"column:password;size:255;index" json:"-"`
	Email           string     `gorm:"column:email;size:255;not null;index" json:"email"`
	Avatar          *string    `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	Language        *string    `gorm:"column:language;size:32;index" json:"language,omitempty"`
	ColorSchema     *string    `gorm:"column:color_schema;size:32;index" json:"color_schema,omitempty"`
	Timezone        *string    `gorm:"column:timezone;size:64;index" json:"timezone,omitempty"`
	LastLoginTime   *time.Time `gorm:"column:last_login_time;index" json:"last_login_time,omitempty"`
	IsAuthenticated string     `gorm:"column:is_authenticated;size:1;not null;default:1;index" json:"is_authenticated"`
	IsActive        string     `gorm:"column:is_active;size:1;not null;default:1;index" json:"is_active"`
	IsAnonymous     string     `gorm:"column:is_anonymous;size:1;not null;default:0;index" json:"is_anonymous"`
	LoginChannel    *string    `gorm:"column:login_channel;index" json:"login_channel,omitempty"`
	Status          *string    `gorm:"column:status;size:1;default:1;index" json:"status"`
	IsSuperuser     *bool      `gorm:"column:is_superuser;index" json:"is_superuser,omitempty"`
	BaseModel
}

// TableName specify table name
func (User) TableName() string {
	return "user"
}
