package models

// UserGear represents the user's gear information.
type UserGear struct {
	UserGearRegistrationID *int64 `json:"usergear_registration_id" db:"user_gear_registrations.userGearRegistrationId"`
	UserGearGearID         int64  `json:"usergear_gear_id" db:"user_gear_registrations.gearId"`
	UserGearUserID         int64  `json:"usergear_user_id" db:"user_gear_registrations.userId"`

	UserID       int64  `json:"user_id" db:"users.userId"`
	UserUsername string `json:"user_username" db:"users.userUsername"`
	UserName     string `json:"user_name" db:"users.userName"`

	GearID            int64  `json:"gear_id" db:"gear.gearId"`
	GearTopCategoryID int64  `json:"gear_top_category_id" db:"gear.gearTopCategoryId"`
	GearCategoryID    int64  `json:"gear_category_id" db:"gear.gearCategoryId"`
	GearManufactureID int64  `json:"gear_manufacture_id" db:"gear.gearManufactureId"`
	GearName          string `json:"gear_name" db:"gear.gearName"`
	GearWeight        int32  `json:"gear_weight" db:"gear.gearWeight"`
	GearHeight        int32  `json:"gear_height" db:"gear.gearHeight"`
	GearLength        int32  `json:"gear_length" db:"gear.gearLength"`
	GearWidth         int32  `json:"gear_width" db:"gear.gearWidth"`
	GearStatus        bool   `json:"gear_status" db:"gear.gearStatus"`
	GearIsContainer   bool   `json:"gear_is_container" db:"gear.gearIsContainer"`
	ContainerLinkID   *int64 `json:"container_link_id" db:"user_container_registration.containerRegistrationId"`
	ContainerID       *int64 `json:"container_registration_id" db:"user_container_registration.userContainerId"`

	ManufactureID   int64  `json:"manufacture_id" db:"manufacture.manufactureId"`
	ManufactureName string `json:"manufacture_name" db:"manufacture.manufactureName"`

	TopCategoryID   int64  `json:"top_category_id" db:"gear_top_category.topCategoryId"`
	TopCategoryName string `json:"top_category_name" db:"gear_top_category.topCategoryName"`

	CategoryID            int64  `json:"category_id" db:"gear_category.categoryId"`
	CategoryName          string `json:"category_name" db:"gear_category.categoryName"`
	CategoryTopCategoryID int64  `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
}

// UserGearLink represents the link between a user and their gear.
type UserGearLink struct {
	UserGearRegistrationID *int64 `json:"usergear_registration_id" db:"userGearRegistrationId"`
	UserGearGearID         int64  `json:"usergear_gear_id" db:"gearId"`
	UserGearUserID         int64  `json:"usergear_user_id" db:"userId"`
}

// UserGearLinkNoID represents the link between a user and their gear without an ID.
type UserGearLinkNoID struct {
	UserGearGearID int64 `json:"usergear_gear_id" db:"gearId"`
	UserGearUserID int64 `json:"usergear_user_id" db:"userId"`
}
