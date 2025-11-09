package models

// Store represents a collection of Gear.
type Store []Gear

// Gear represents a piece of gear.
type Gear struct {
	GearID             *int64 `json:"gear_id" db:"gearId"`
	GearTopCategoryID  int64  `json:"gear_top_category_id" db:"gearTopCategoryId"`
	GearCategoryID     int64  `json:"gear_category_id" db:"gearCategoryId"`
	GearManufactureID  int64  `json:"gear_manufacture_id" db:"gearManufactureId"`
	GearIsContainer    bool   `json:"gear_is_container" db:"gearIsContainer"`
	GearName           string `json:"gear_name" db:"gearName"`
	GearSizeDefinition string `json:"gear_size_definition" db:"gearSizeDefinition"`
	GearWeight         int32  `json:"gear_weight" db:"gearWeight"`
	GearHeight         int32  `json:"gear_height" db:"gearHeight"`
	GearLength         int32  `json:"gear_length" db:"gearLength"`
	GearWidth          int32  `json:"gear_width" db:"gearWidth"`
	GearStatus         bool   `json:"gear_status" db:"gearStatus"`
}

// GearNameList represents a list of gear names.
type GearNameList struct {
	GearID   string `json:"gear_id" db:"gearId"`
	GearName string `json:"gear_name" db:"gearName"`
}

// GearNoID represents a piece of gear without an ID.
type GearNoID struct {
	GearTopCategoryID  int64  `json:"gear_top_category_id" db:"gearTopCategoryId"`
	GearCategoryID     int64  `json:"gear_category_id" db:"gearCategoryId"`
	GearManufactureID  int64  `json:"gear_manufacture_id" db:"gearManufactureId"`
	GearIsContainer    bool   `json:"gear_is_container" db:"gearIsContainer"`
	GearName           string `json:"gear_name" db:"gearName"`
	GearSizeDefinition string `json:"gear_size_definition" db:"gearSizeDefinition"`
	GearWeight         int32  `json:"gear_weight" db:"gearWeight"`
	GearHeight         int32  `json:"gear_height" db:"gearHeight"`
	GearLength         int32  `json:"gear_length" db:"gearLength"`
	GearWidth          int32  `json:"gear_width" db:"gearWidth"`
	GearStatus         bool   `json:"gear_status" db:"gearStatus"`
}

// FullGear represents a complete gear object.
type FullGear struct {
	GearID             int64  `json:"gear_id" db:"gear.gearId"`
	GearTopCategoryID  int64  `json:"gear_top_category_id" db:"gear.gearTopCategoryId"`
	GearCategoryID     int64  `json:"gear_category_id" db:"gear.gearCategoryId"`
	GearManufactureID  int64  `json:"gear_manufacture_id" db:"gear.gearManufactureId"`
	GearIsContainer    bool   `json:"gear_is_container" db:"gearIsContainer"`
	GearName           string `json:"gear_name" db:"gear.gearName"`
	GearSizeDefinition string `json:"gear_size_definition" db:"gearSizeDefinition"`
	GearWeight         int32  `json:"gear_weight" db:"gear.gearWeight"`
	GearHeight         int32  `json:"gear_height" db:"gear.gearHeight"`
	GearLength         int32  `json:"gear_length" db:"gear.gearLength"`
	GearWidth          int32  `json:"gear_width" db:"gear.gearWidth"`
	GearStatus         bool   `json:"gear_status" db:"gear.gearStatus"`

	ManufactureID   int64  `json:"manufacture_id" db:"manufacture.manufactureId"`
	ManufactureName string `json:"manufacture_name" db:"manufacture.manufactureName"`

	TopCategoryID   int64  `json:"top_category_id" db:"gear_top_category.topCategoryId"`
	TopCategoryName string `json:"top_category_name" db:"gear_top_category.topCategoryName"`

	CategoryID            int64  `json:"category_id" db:"gear_category.categoryId"`
	CategoryName          string `json:"category_name" db:"gear_category.categoryName"`
	CategoryTopCategoryID int64  `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
}

// GearListItem represents a gear list item.
type GearListItem struct {
	GearID                int64  `json:"gear_id" db:"gear.gearId"`
	GearTopCategoryID     int64  `json:"gear_top_category_id" db:"gear.gearTopCategoryId"`
	GearCategoryID        int64  `json:"gear_category_id" db:"gear.gearCategoryId"`
	GearManufactureID     int64  `json:"gear_manufacture_id" db:"gear.gearManufactureId"`
	GearIsContainer       bool   `json:"gear_is_container" db:"gearIsContainer"`
	GearName              string `json:"gear_name" db:"gear.gearName"`
	GearSizeDefinition    string `json:"gear_size_definition" db:"gearSizeDefinition"`
	ManufactureID         int64  `json:"manufacture_id" db:"manufacture.manufactureId"`
	ManufactureName       string `json:"manufacture_name" db:"manufacture.manufactureName"`
	TopCategoryID         int64  `json:"top_category_id" db:"gear_top_category.topCategoryId"`
	TopCategoryName       string `json:"top_category_name" db:"gear_top_category.topCategoryName"`
	CategoryID            int64  `json:"category_id" db:"gear_category.categoryId"`
	CategoryName          string `json:"category_name" db:"gear_category.categoryName"`
	CategoryTopCategoryID int64  `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
}

// Measurement represents a measurement value with its unit.
type Measurement struct {
	Value int16 `json:"value" db:"value"`
	Unit  bool  `json:"unit" db:"unit" validate:"oneof=inches feet centimeter meter grams kilos pounds"`
}
