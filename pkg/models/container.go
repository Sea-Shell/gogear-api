package models

// UserContainer represents the link between a user and their Container.
type UserContainer struct {
	ContainerRegistrationID *int64 `json:"container_registration_id" db:"containerRegistrationId"`
	UserContainerID         int64  `json:"user_container_id" db:"userContainerId"`
	UserGearRegistrationID  int64  `json:"user_gear_registration_id" db:"userGearRegistrationId"`
}

// UserContainerNoID represents the link between a user and their Container without an ID.
type UserContainerNoID struct {
	UserContainerID        int64 `json:"user_container_id" db:"userContainerId"`
	UserGearRegistrationID int64 `json:"user_gear_registration_id" db:"userGearRegistrationId"`
}
