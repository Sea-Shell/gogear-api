package models

// LoadoutPlace is a place of a loadout (ARS-001 section 3): the built-in Worn,
// the optional Last minute, or a bag (one of the owner's container
// registrations). Bag-only fields are null for Worn and Last minute.
type LoadoutPlace struct {
	PlaceID                int64   `json:"place_id"`
	LoadoutID              int64   `json:"loadout_id"`
	PlaceKind              string  `json:"place_kind"` // "worn", "last_minute" or "bag"
	Closed                 bool    `json:"closed"`
	UsergearRegistrationID *int64  `json:"usergear_registration_id"`
	GearID                 *int64  `json:"gear_id"`
	GearName               *string `json:"gear_name"`
	GearIsContainer        *bool   `json:"gear_is_container"`
	MaxContainerWeight     *int64  `json:"max_container_weight"`
	CatalogWeight          *int64  `json:"catalog_weight"`
	UnitWeight             *int64  `json:"unit_weight"`
	WeightSource           *string `json:"weight_source"`
	// The bag registration's own weight, null when it has none (BR-026).
	MeasuredWeight       *int64  `json:"measured_weight"`
	MeasuredWeightSource *string `json:"measured_weight_source"`
	ParentPlaceID        *int64  `json:"parent_place_id"`
	Position             *int64  `json:"position"`
}

// LoadoutPlaceInsert is the body of the place insert request. place_kind is
// "bag" (usergear_registration_id required; parent_place_id optional: absent =
// the closet default, null = top level) or "last_minute".
type LoadoutPlaceInsert struct {
	PlaceKind              string `json:"place_kind"`
	UsergearRegistrationID *int64 `json:"usergear_registration_id,omitempty"`
	ParentPlaceID          *int64 `json:"parent_place_id,omitempty"`
}

// LoadoutPlaceUpdate is the body of the place update request. parent_place_id
// is required: a number puts the bag inside that bag, null makes it top level.
type LoadoutPlaceUpdate struct {
	ParentPlaceID *int64 `json:"parent_place_id"`
}

// LoadoutPlaceOrder is the body of the place order request: every bag of the
// loadout exactly once, in the new order.
type LoadoutPlaceOrder struct {
	PlaceIDs []int64 `json:"place_ids"`
}

// LoadoutPlaceClose is the body of the place close request.
type LoadoutPlaceClose struct {
	Closed bool `json:"closed"`
}

// LoadoutPlaceCloseResult is the response of the place close request.
type LoadoutPlaceCloseResult struct {
	PlaceID int64 `json:"place_id"`
	Closed  bool  `json:"closed"`
}
