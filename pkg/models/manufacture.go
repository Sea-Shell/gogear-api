package models

// Manufacture represents a gear manufacture.
type Manufacture struct {
    ManufactureID   *int64 `json:"manufacture_id" db:"manufactureId"`
    ManufactureName string `json:"manufacture_name" db:"manufactureName"`
}
