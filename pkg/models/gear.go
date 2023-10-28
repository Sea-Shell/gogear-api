package models

type Store []Gear

type Gear struct {
    GearId            *int64  `json:"gear_id" db:"gearId"`
    GearTopCategoryId int64   `json:"gear_top_category_id" db:"gearTopCategoryId"`
    GearCategoryId    int64   `json:"gear_category_id" db:"gearCategoryId"`
    GearManufactureId int64   `json:"gear_manufacture_id" db:"gearManufactureId"`
    GearName          string  `json:"gear_name" db:"gearName"`
    GearWeight        int32   `json:"gear_weight" db:"gearWeight"`
    GearHeight        int32   `json:"gear_height" db:"gearHeight"`
    GearLength        int32   `json:"gear_length" db:"gearLength"`
    GearWidth         int32   `json:"gear_width" db:"gearWidth"`
    GearStatus        bool    `json:"gear_status" db:"gearStatus"`
}

type GearNameList struct {
    GearId      string  `json:"gear_id" db:"gearId"`
    GearName    string  `json:"gear_name" db:"gearName"`
}

type GearNoId struct {
    GearTopCategoryId int64  `json:"gear_top_category_id" db:"gearTopCategoryId"`
    GearCategoryId    int64  `json:"gear_category_id" db:"gearCategoryId"`
    GearManufactureId int64  `json:"gear_manufacture_id" db:"gearManufactureId"`
    GearName          string `json:"gear_name" db:"gearName"`
    GearWeight        int32  `json:"gear_weight" db:"gearWeight"`
    GearHeight        int32  `json:"gear_height" db:"gearHeight"`
    GearLength        int32  `json:"gear_length" db:"gearLength"`
    GearWidth         int32  `json:"gear_width" db:"gearWidth"`
    GearStatus        bool   `json:"gear_status" db:"gearStatus"`
}

type FullGear struct {
    GearId                int64  `json:"gear_id" db:"gear.gearId"`
    GearTopCategoryId     int64  `json:"gear_top_category_id" db:"gear.gearTopCategoryId"`
    GearCategoryId        int64  `json:"gear_category_id" db:"gear.gearCategoryId"`
    GearManufactureId     int64  `json:"gear_manufacture_id" db:"gear.gearManufactureId"`
    GearName              string `json:"gear_name" db:"gear.gearName"`
    GearWeight            int32  `json:"gear_weight" db:"gear.gearWeight"`
    GearHeight            int32  `json:"gear_height" db:"gear.gearHeight"`
    GearLength            int32  `json:"gear_length" db:"gear.gearLength"`
    GearWidth             int32  `json:"gear_width" db:"gear.gearWidth"`
    GearStatus            bool   `json:"gear_status" db:"gear.gearStatus"`
    
    ManufactureId         int64  `json:"manufacture_id" db:"manufacture.manufactureId"`
    ManufactureName       string `json:"manufacture_name" db:"manufacture.manufactureName"`
    
    TopCategoryId         int64  `json:"top_category_id" db:"gear_top_category.topCategoryId"`
    TopCategoryName       string `json:"top_category_name" db:"gear_top_category.topCategoryName"`
    
    CategoryId            int64  `json:"category_id" db:"gear_category.categoryId"`
    CategoryName          string `json:"category_name" db:"gear_category.categoryName"`
    CategoryTopCategoryId int64 `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
}

type GearListItem struct {
    GearId                int64  `json:"gear_id" db:"gear.gearId"`
    GearTopCategoryId     int64  `json:"gear_top_category_id" db:"gear.gearTopCategoryId"`
    GearCategoryId        int64  `json:"gear_category_id" db:"gear.gearCategoryId"`
    GearManufactureId     int64  `json:"gear_manufacture_id" db:"gear.gearManufactureId"`
    GearName              string `json:"gear_name" db:"gear.gearName"`
    ManufactureId         int64  `json:"manufacture_id" db:"manufacture.manufactureId"`
    ManufactureName       string `json:"manufacture_name" db:"manufacture.manufactureName"`
    TopCategoryId         int64  `json:"top_category_id" db:"gear_top_category.topCategoryId"`
    TopCategoryName       string `json:"top_category_name" db:"gear_top_category.topCategoryName"`
    CategoryId            int64  `json:"category_id" db:"gear_category.categoryId"`
    CategoryName          string `json:"category_name" db:"gear_category.categoryName"`
    CategoryTopCategoryId int64 `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
}

type Measurement struct {
    Value int16 `json:"value" db:"value"`
    Unit  bool  `json:"unit" db:"unit" validate:"oneof=inches feet centimeter meter grams kilos pounds"`
}