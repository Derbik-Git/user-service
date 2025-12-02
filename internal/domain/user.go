package domain

import "time"

type User struct {
	ID        int64
	Email     string
	Name      string
	CreatedAt time.Time
}
