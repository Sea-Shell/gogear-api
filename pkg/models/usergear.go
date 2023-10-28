package models

type UserGear struct {
    UserGearRegistrationId  *int64      `json:"usergear_registration_id" db:"user_gear_registrations.userGearRegistrationId"`
    UserGearGearId          int64       `json:"usergear_gear_id" db:"user_gear_registrations.gearId"`
    UserGearUserId          int64       `json:"usergear_user_id" db:"user_gear_registrations.userId"`

    UserId                  int64       `json:"user_id" db:"users.userId"`
    UserUsername            string      `json:"user_username" db:"users.userUsername"`
    UserName                string      `json:"user_name" db:"users.userName"`
    
    GearId                  int64       `json:"gear_id" db:"gear.gearId"`
    GearTopCategoryId       int64       `json:"gear_top_category_id" db:"gear.gearTopCategoryId"`
    GearCategoryId          int64       `json:"gear_category_id" db:"gear.gearCategoryId"`
    GearManufactureId       int64       `json:"gear_manufacture_id" db:"gear.gearManufactureId"`
    GearName                string      `json:"gear_name" db:"gear.gearName"`
    GearWeight              int32       `json:"gear_weight" db:"gear.gearWeight"`
    GearHeight              int32       `json:"gear_height" db:"gear.gearHeight"`
    GearLength              int32       `json:"gear_length" db:"gear.gearLength"`
    GearWidth               int32       `json:"gear_width" db:"gear.gearWidth"`
    GearStatus              bool        `json:"gear_status" db:"gear.gearStatus"`
    
    ManufactureId           int64       `json:"manufacture_id" db:"manufacture.manufactureId"`
    ManufactureName         string      `json:"manufacture_name" db:"manufacture.manufactureName"`
    
    TopCategoryId           int64       `json:"top_category_id" db:"gear_top_category.topCategoryId"`
    TopCategoryName         string      `json:"top_category_name" db:"gear_top_category.topCategoryName"`
    
    CategoryId              int64       `json:"category_id" db:"gear_category.categoryId"`
    CategoryName            string      `json:"category_name" db:"gear_category.categoryName"`
    CategoryTopCategoryId   int64       `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
}

type UserGearLink struct {
    UserGearRegistrationId  *int64      `json:"usergear_registration_id" db:"userGearRegistrationId"`
    UserGearGearId          int64       `json:"usergear_gear_id" db:"gearId"`
    UserGearUserId          int64       `json:"usergear_user_id" db:"userId"`
}

type UserGearLinkNoId struct {
    UserGearGearId          int64       `json:"usergear_gear_id" db:"gearId"`
    UserGearUserId          int64       `json:"usergear_user_id" db:"userId"`
}