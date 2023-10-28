package models

type GearTopCategory struct {
    TopCategoryId   *int64  `json:"top_category_id" db:"topCategoryId"`
    TopCategoryName string  `json:"top_category_name" db:"topCategoryName"`
}
