package access

// Keep automatic delivery and the administrator's resend action identical.
func completionNotice(r Request) notice {
	return notice{r.Email, "Emby setup complete - you're ready to watch", `Hi ` + r.Name + `,

Your setup is complete. Your Emby Connect account is linked and your server account is ready to use.

Emby Connect account: ` + r.Connect + `

Use your Emby Connect password when choosing Emby Connect sign-in. This is separate from the local server password you chose during setup. If an app asks for a local server login instead, your username is ` + r.Username + ` and the password is the one you set during setup.

TVs and streaming devices
Install the Emby app for your supported TV or streaming device (Roku, Fire TV, Android/Google TV, Apple TV, Samsung or LG). Choose Emby Connect. If a code appears, open the address shown on the TV on your phone or computer, sign in to Emby Connect, and enter that code. Return to the TV and select the linked server. Follow the app's prompts if it offers direct sign-in instead.

Web browser
Open https://app.emby.media, sign in with Emby Connect, and select the linked server.

Phones and tablets
Install Emby from the Apple App Store or Google Play. Open it, choose Emby Connect, sign in, and select the linked server.

Gaming consoles
Xbox One / Series X|S: install Emby from the Microsoft Store, open it, and choose Emby Connect. Follow the on-screen sign-in or code instructions, then select the server.
PlayStation 4: open https://tv.emby.media in the console's web browser and follow the sign-in prompts. You can bookmark it for next time.
Other consoles: availability varies; check the device list below. If your console is not supported, use a supported TV, streaming device, phone, or browser.

Official apps and device instructions: https://emby.media/download.html
Emby Connect help: https://emby.media/connect.html

If the server does not appear, check that you signed in with the Connect account above. If you still cannot connect, contact the server owner. Never send anyone your password.

Enjoy watching!
`}
}
