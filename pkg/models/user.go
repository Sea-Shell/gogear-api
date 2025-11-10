package models

// UserWithPass represents a user with password.
type UserWithPass struct {
	UserID       *int64 `json:"user_id" db:"userId"`
	UserUsername string `json:"user_username" db:"userUserName"`
	UserPassword string `json:"user_password" db:"userPassword"`
	UserName     string `json:"user_name" db:"userName"`
	UserEmail    string `json:"user_email" db:"userEmail"`
	UserIsAdmin  bool   `json:"user_is_admin" db:"userIsAdmin"`
}

// User represents a user.
type User struct {
	UserID       *int64 `json:"user_id" db:"userId"`
	UserPassword string `json:"-" db:"userPassword"`
	UserUsername string `json:"user_username" db:"userUsername"`
	UserName     string `json:"user_name" db:"userName"`
	UserEmail    string `json:"user_email" db:"userEmail"`
	UserIsAdmin  bool   `json:"user_is_admin" db:"userIsAdmin"`
}

// type UserGear struct {
//     GearID int64  `json:"gear_id" db:"gearId"`
//     UserID int64  `json:"user_id" db:"userId"`
// }

// UserInventory represents a user's inventory.
type UserInventory struct {
	GearID     int64  `json:"gear_id" db:"gear_id"`
	CustomName string `json:"custom_name" db:"custom_name"`
}
