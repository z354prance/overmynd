package access

func isJellyfin(s Settings) bool { return s.Provider == "jellyfin" }

func serverCompletionNotice(r Request, s Settings) notice {
	if !isJellyfin(s) {
		return completionNotice(r)
	}
	return notice{r.Email, "Jellyfin setup complete - you are ready to watch", "Hi " + r.Name + ",\n\nYour Jellyfin account is ready.\n\nServer: " + s.ServerPublicURL + "\nUsername: " + r.Username + "\nPassword: the password you chose during setup.\n\nBrowser: open the server URL above and sign in.\n\nPhones and tablets: install a Jellyfin client from your app store, add the server URL above, and sign in.\n\nTVs and streaming devices: install a supported Jellyfin app, enter the server URL, and sign in. If the client offers Quick Connect, authorize its code from an already signed-in Jellyfin client.\n\nGaming consoles: client availability varies. Check the official client list; use a supported streaming device or browser if no suitable client exists.\n\nOfficial clients: https://jellyfin.org/downloads/clients/\nQuick Connect: https://jellyfin.org/docs/general/server/quick-connect/\n\nContact the server owner if you need help. Never send anyone your password.\n"}
}
