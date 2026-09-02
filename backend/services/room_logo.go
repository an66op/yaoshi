package services

import "strings"

// Keep this in sync with the first RoomLogoPicker preset. An empty stored logo
// means "use the default"; resolve it only for display so existing custom room
// identities and the owner's ability to clear a custom logo remain unchanged.
const defaultRoomLogo = "/images/wangzhe-header-logo.png"

func roomLogoForDisplay(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultRoomLogo
	}
	return value
}
