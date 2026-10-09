package re2

import "strings"

// ChannelData is the data channel: AI session inventory and PTY, independent of
// the desktop-control channel so either can come and go without the other.
const ChannelData = "data"

const channelSep = "#"

// ChannelRoute is the frame RouteID for a device's channel. The default
// (desktop) channel keeps the bare device id for older peers.
func ChannelRoute(deviceID, channel string) string {
	if channel == "" {
		return deviceID
	}
	return deviceID + channelSep + channel
}

// SplitRoute is the inverse of ChannelRoute.
func SplitRoute(route string) (deviceID, channel string) {
	if i := strings.LastIndex(route, channelSep); i >= 0 {
		return route[:i], route[i+len(channelSep):]
	}
	return route, ""
}
