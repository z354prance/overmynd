package access

import (
	"fmt"
	"net/url"
	"strings"
)

func siteHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

func siteName(s Settings) string {
	if u, err := url.Parse(s.PublicURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "the server"
}

func receiptNotice(r Request, s Settings) notice {
	return notice{r.Email, "We got your access request", letter{
		Title:   "We got your request",
		Intro:   []string{"Hi " + r.Name + ", your request for an account on " + siteName(s) + " is in. An administrator will review it, and no account has been created yet."},
		Details: [][2]string{{"Requested username", r.Username}, {"Notification email", r.Email}},
		Parts:   [][2]string{{"What happens next", "You will get another email when the request is approved, with a link to finish setting up your account. If it is declined, we will tell you that too."}},
		Note:    "You did not need to do anything else. If you did not make this request, you can ignore this email.",
	}}
}

func adminNewNotice(r Request, s Settings) notice {
	d := [][2]string{{"Name", r.Name}, {"Email", r.Email}, {"Username", r.Username}}
	if r.Connect != "" {
		d = append(d, [2]string{"Emby Connect email", r.Connect})
	}
	return notice{s.AdminEmail, "New access request from " + r.Name, letter{
		Title:   "New access request",
		Intro:   []string{r.Name + " asked for an account. Approving it creates a disabled copy of your template user and emails them a single-use setup link."},
		Details: d,
		Button:  [2]string{"Review the request", s.PublicURL + "/admin"},
	}}
}

func adminPendingNotice(r Request, s Settings) notice {
	n := adminNewNotice(r, s)
	n.subject = "Access request waiting: " + r.Name
	n.l.Title = "This request is still waiting"
	return n
}

func setupNotice(r Request, s Settings, token string) notice {
	link := s.PublicURL + "/#access-setup=" + token
	if isJellyfin(s) {
		return notice{r.Email, "Your account is approved - choose your password", letter{
			Title:   "You're approved",
			Intro:   []string{"Hi " + r.Name + ", your Jellyfin account has been created. It stays switched off until you choose a password."},
			Details: [][2]string{{"Your username", r.Username}},
			Steps:   []string{"Open the setup page with the button below.", "Choose a password of at least 12 characters.", "Sign in to Jellyfin with your username and that password."},
			Button:  [2]string{"Choose your password", link},
			Note:    "This link works once and expires in 24 hours. Do not share it. If it expires, ask the server owner to send a new one.",
		}}
	}
	return notice{r.Email, "Your account is approved - finish setting it up", letter{
		Title:   "You're approved",
		Intro:   []string{"Hi " + r.Name + ", your Emby account has been created. It stays switched off until you finish setup."},
		Details: [][2]string{{"Your username", r.Username}, {"Emby Connect email", r.Connect}},
		Steps: []string{
			"Emby will email you (at the Connect address above) to confirm linking your Emby Connect account. Confirm it first.",
			"Open the setup page with the button below.",
			"Choose a password of at least 12 characters. This is separate from your Emby Connect password.",
			"Sign in with Emby Connect, or with your username and this password.",
		},
		Button: [2]string{"Finish setting up", link},
		Note:   "This link works once and expires in 24 hours. Do not share it. Undermynd will never ask for your Emby Connect password. If the link expires, ask the server owner to send a new one.",
	}}
}

func declinedNotice(r Request) notice {
	return notice{r.Email, "About your access request", letter{
		Title: "Your request was not approved",
		Intro: []string{"Hi " + r.Name + ", the server owner did not approve your request for an account this time. No account was created.", "If you think this was a mistake, contact the server owner directly."},
	}}
}

func adminProblemNotice(id int64, s Settings, message string) notice {
	return notice{s.AdminEmail, "Access request needs attention", letter{
		Title:   "An account setup needs attention",
		Intro:   []string{fmt.Sprintf("Request %d did not finish, and the account has been left switched off.", id), message},
		Details: [][2]string{{"Request", fmt.Sprint(id)}},
		Parts:   [][2]string{{"What to do", "Open the request in the admin page. You can correct the details and retry, send a fresh setup link, or reapply the template."}},
		Button:  [2]string{"Open the admin page", s.PublicURL + "/admin"},
	}}
}

func adminStuckNotice(id int64, s Settings, message string) notice {
	n := adminProblemNotice(id, s, message)
	n.subject = "A new user could not finish setup"
	n.l.Title = "A new user could not finish setup"
	n.l.Intro = []string{fmt.Sprintf("The person on request %d tried to finish setup and it failed. Their account is switched off and their link still works.", id), message}
	return n
}

func testNotice(s Settings) notice {
	return notice{s.AdminEmail, "Undermynd email test", letter{
		Title:   "Email is working",
		Intro:   []string{"This is a test from Undermynd. Account request emails will be delivered like this one."},
		Details: [][2]string{{"Sent from", s.From}, {"Server", s.SMTPHost}},
	}}
}

func embyCompletionNotice(r Request, s Settings) notice {
	details := [][2]string{{"Emby Connect email", r.Connect}, {"Server username", r.Username}}
	web := "Open https://app.emby.media, sign in with Emby Connect, and select the linked server."
	var button [2]string
	if s.ServerPublicURL != "" {
		details = append([][2]string{{"Watch on the web", s.ServerPublicURL}}, details...)
		web = "Open " + s.ServerPublicURL + " in any browser and sign in. You can also use https://app.emby.media with Emby Connect and select the linked server."
		button = [2]string{"Open " + siteHost(s.ServerPublicURL), s.ServerPublicURL}
	}
	return notice{r.Email, "You're ready to watch", letter{
		Title:   "Your account is ready",
		Intro:   []string{"Hi " + r.Name + ", setup is complete. Your Emby Connect account is linked and your server account is ready to use."},
		Details: details,
		Button:  button,
		Parts: [][2]string{
			{"Which password to use", "Use your Emby Connect password when you choose Emby Connect sign-in. If an app asks for a local server login instead, use your server username with the password you chose during setup."},
			{"TVs and streaming devices", "Install the Emby app (Roku, Fire TV, Android or Google TV, Apple TV, Samsung or LG) and choose Emby Connect. If a code appears, open the address shown on the TV on your phone or computer, sign in to Emby Connect, and enter the code. Then select the linked server."},
			{"Web browser", web},
			{"Phones and tablets", "Install Emby from the Apple App Store or Google Play, choose Emby Connect, sign in, and select the linked server."},
			{"Gaming consoles", "Xbox One and Series X|S: install Emby from the Microsoft Store and choose Emby Connect. PlayStation 4: open https://tv.emby.media in the console's browser and follow the prompts. Other consoles vary; if yours is not supported, use a TV, streaming device, phone or browser."},
		},
		Links: [][2]string{{"Emby apps and devices", "https://emby.media/download.html"}, {"Emby Connect help", "https://emby.media/connect.html"}},
		Note:  "If the server does not appear, check that you signed in with the Emby Connect account above. If you still cannot connect, contact the server owner. Never send anyone your password.",
	}}
}

func jellyfinCompletionNotice(r Request, s Settings) notice {
	return notice{r.Email, "You're ready to watch", letter{
		Title:   "Your account is ready",
		Intro:   []string{"Hi " + r.Name + ", your Jellyfin account is ready to use."},
		Details: [][2]string{{"Server", s.ServerPublicURL}, {"Username", r.Username}, {"Password", "the one you chose during setup"}},
		Parts: [][2]string{
			{"Web browser", "Open the server address above and sign in."},
			{"Phones and tablets", "Install a Jellyfin app from your app store, add the server address, and sign in."},
			{"TVs and streaming devices", "Install a supported Jellyfin app, enter the server address, and sign in. If the app offers Quick Connect, approve its code from a Jellyfin app you are already signed in to."},
			{"Gaming consoles", "Apps vary by console. Check the official list, or use a streaming device or browser if yours is not supported."},
		},
		Links: [][2]string{{"Jellyfin apps", "https://jellyfin.org/downloads/clients/"}, {"About Quick Connect", "https://jellyfin.org/docs/general/server/quick-connect/"}},
		Note:  "Contact the server owner if you need help. Never send anyone your password.",
	}}
}

var _ = strings.TrimSpace
