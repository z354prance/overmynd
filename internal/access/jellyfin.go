package access

func isJellyfin(s Settings) bool { return s.Provider == "jellyfin" }

func serverCompletionNotice(r Request, s Settings) notice {
	if !isJellyfin(s) {
		return embyCompletionNotice(r, s)
	}
	return jellyfinCompletionNotice(r, s)
}
