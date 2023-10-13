package models

type Manufacture struct {
	ManufactureId *int64  `json:"manufacture_id" db:"manufactureId"`
	ManufactureName          string `json:"manufacture_name" db:"manufactureName"`
}