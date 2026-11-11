package room

import "github.com/hilthontt/visper/services/api/domain/apperr"

var (
	ErrRoomIDRequired      = apperr.New(apperr.Invalid, "room ID cannot be empty")
	ErrUserIDRequired      = apperr.New(apperr.Invalid, "user ID cannot be empty")
	ErrJoinCodeRequired    = apperr.New(apperr.Invalid, "join code cannot be empty")
	ErrSecureTokenRequired = apperr.New(apperr.Invalid, "secure token cannot be empty")

	ErrRoomNotFound = apperr.New(apperr.NotFound, "room not found")
	ErrRoomExpired  = apperr.New(apperr.NotFound, "room has expired")
	ErrNotMember    = apperr.New(apperr.NotFound, "user is not a member of this room")

	ErrOwnerOnlyUpdate     = apperr.New(apperr.Forbidden, "only the room owner can update the room")
	ErrOwnerOnlyDelete     = apperr.New(apperr.Forbidden, "only the room owner can delete the room")
	ErrOwnerOnlyKick       = apperr.New(apperr.Forbidden, "only the room owner can kick members")
	ErrOwnerCannotLeave    = apperr.New(apperr.Forbidden, "room owner cannot leave, delete the room instead")
	ErrOwnerCannotBeKicked = apperr.New(apperr.Forbidden, "room owner cannot be kicked, delete the room instead")
	ErrInvalidCodes        = apperr.New(apperr.Forbidden, "invalid join code or secure token")
	ErrInvalidSecureToken  = apperr.New(apperr.Forbidden, "invalid secure token")
)

// errJoinCodeNotFound reports that no room uses joinCode.
func errJoinCodeNotFound(joinCode string) error {
	return apperr.Newf(apperr.NotFound, "room not found with join code: %s", joinCode)
}
