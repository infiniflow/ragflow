package entity

import "time"

// AdminOperationLog records account operations without request bodies or credentials.
type AdminOperationLog struct {
	ID          string    `gorm:"column:id;size:32;primaryKey" json:"id"`
	EventTime   time.Time `gorm:"column:event_time;not null;index" json:"event_time"`
	ActorID     string    `gorm:"column:actor_id;size:32;index" json:"actor_id"`
	ActorEmail  string    `gorm:"column:actor_email;size:255;index" json:"actor_email"`
	TargetID    string    `gorm:"column:target_id;size:32;index" json:"target_id"`
	TargetEmail string    `gorm:"column:target_email;size:255;index" json:"target_email"`
	Action      string    `gorm:"column:action;size:64;not null;index" json:"action"`
	APIPath     string    `gorm:"column:api_path;size:255;not null" json:"api_path"`
	IPAddress   string    `gorm:"column:ip_address;size:64" json:"ip_address"`
	Result      string    `gorm:"column:result;size:16;not null;index" json:"result"`
	HTTPStatus  int       `gorm:"column:http_status" json:"http_status"`
	ResultCode  int       `gorm:"column:result_code" json:"result_code"`
	DurationMS  int64     `gorm:"column:duration_ms" json:"duration_ms"`
}

func (AdminOperationLog) TableName() string {
	return "admin_operation_log"
}
