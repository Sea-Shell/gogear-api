package models

type UserWithPass struct {
    UserId          *int64  `json:"user_id" db:"userId"`
    UserUsername    string  `json:"user_username" db:"userUserName"`
    UserPassword    string  `json:"user_password" db:"userPassword"`
    UserName        string  `json:"user_name" db:"userName"`
    UserEmail       string  `json:"user_email" db:"userEmail"`
}

type User struct {
    UserId          *int64  `json:"user_id" db:"userId"`
    UserPassword    string  `json:"-" db:"userPassword"`
    UserUsername    string  `json:"user_username" db:"userUsername"`
    UserName        string  `json:"user_name" db:"userName"`
    UserEmail       string  `json:"user_email" db:"userEmail"`
}

// type UserGear struct {
// 	GearId int64  `json:"gear_id" db:"gearId"`
// 	UserId int64  `json:"user_id" db:"userId"`
// }

type UserInventory struct {
    GearId      int64   `json:"gear_id" db:"gear_id"`
    CustomName  string  `json:"custom_name" db:"custom_name"`
}
