// Package config holds environment settings and the editable settings file.
package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/secret"
)

// Env are process-level settings that come from the container environment.
type Env struct {
	ConfigDir    string
	DataDir      string
	WorkDir      string // staging, Tdarr hand-off and local client folders; default <data>/undermynd
	CompleteDir  string // where finished jobs are handed to the arrs; default <data>/complete
	PublicAddr   string
	MachineAddr  string
	TrustProxy   bool
	LogRequests  bool
	TickInterval time.Duration
}

// EnvValue prefers the Overmynd spelling while preserving all existing
// Undermynd deployment settings. An explicitly empty new variable overrides it.
func EnvValue(legacy string) string {
	modern := strings.Replace(legacy, "UNDERMYND_", "OVERMYND_", 1)
	if value, ok := os.LookupEnv(modern); ok {
		return value
	}
	return os.Getenv(legacy)
}

func LoadEnv() Env {
	get := func(k, d string) string {
		if v := EnvValue(k); v != "" {
			return v
		}
		return d
	}
	tick := 5 * time.Second
	if d, err := time.ParseDuration(EnvValue("UNDERMYND_TICK")); err == nil && d > 0 {
		tick = d
	}
	return Env{
		ConfigDir:    get("UNDERMYND_CONFIG", "/config"),
		DataDir:      get("UNDERMYND_DATA", "/data"),
		WorkDir:      EnvValue("UNDERMYND_WORK"),
		CompleteDir:  EnvValue("UNDERMYND_COMPLETE"),
		PublicAddr:   get("UNDERMYND_PUBLIC_ADDR", ":8080"),
		MachineAddr:  get("UNDERMYND_MACHINE_ADDR", ":8081"),
		TrustProxy:   EnvValue("UNDERMYND_TRUST_PROXY") == "true",
		LogRequests:  EnvValue("UNDERMYND_LOG_REQUESTS") == "1",
		TickInterval: tick,
	}
}

// Category maps a download-client category (what the arr sends) to behaviour.
type Category struct {
	Name      string        `json:"name"`
	Kind      string        `json:"kind"` // radarr | sonarr | lidarr | rss
	ArrURL    string        `json:"arrUrl"`
	ArrAPIKey secret.String `json:"arrApiKey"`
	Transcode bool          `json:"transcode"`
	// LibraryDir, for categories no arr handles, is a library of show folders the
	// finished episodes are placed into, matched by title.
	LibraryDir string `json:"libraryDir,omitempty"`
	// OnTranscodeFail is "fail" (default: the arr blocklists and searches again) or "passthrough" (hand back the original).
	OnTranscodeFail string `json:"onTranscodeFail"`
}

type QBit struct {
	URL      string        `json:"url"`
	Username string        `json:"username"`
	Password secret.String `json:"password"`
	// SaveDir is where the local client saves each job, as seen by both the
	// client and this container (the shared /data mount).
	SaveDir string `json:"saveDir"`
}

type TorBox struct {
	Token          secret.String `json:"token"`
	BaseURL        string        `json:"baseUrl"`
	CreatePerHour  int           `json:"createPerHour"`
	PullersAtOnce  int           `json:"pullersAtOnce"`
	LinkGapSeconds int           `json:"linkGapSeconds"`
}

type NZBGet struct {
	URL      string        `json:"url"`
	Username string        `json:"username"`
	Password secret.String `json:"password"`
	// DestRoot is NZBGet's destination folder, the same path in both containers.
	DestRoot string `json:"destRoot"`
}

// Tdarr hands videos to a Tdarr library by folder. Create a library in Tdarr that
// watches InDir and writes to OutDir (folder to folder); Overmynd does not touch any other library.
type Tdarr struct {
	URL            string        `json:"url"` // optional; with it Overmynd watches Tdarr over its API
	APIKey         secret.String `json:"apiKey"`
	LibraryID      string        `json:"libraryId"` // the dedicated Overmynd library
	MaxInFlight    int           `json:"maxInFlight"`
	StallMinutes   int           `json:"stallMinutes"`
	InDir          string        `json:"inDir"`
	OutDir         string        `json:"outDir"`
	TimeoutMinutes int           `json:"timeoutMinutes"`
}

// Playback feeds the public Now Playing and Recently Added from Tracearr; Emby is
// only used to hide whole libraries from Now Playing.
type Playback struct {
	TracearrURL     string        `json:"tracearrUrl"`
	TracearrKey     secret.String `json:"tracearrKey"`
	EmbyURL         string        `json:"embyUrl"`
	EmbyKey         secret.String `json:"embyKey"`
	HiddenLibraries string        `json:"hiddenLibraries"` // comma-separated Emby library IDs
}

// Donations are the links behind the public Donate button; blank links hide it.
type Donations struct {
	Title       string `json:"title"`
	PayPalURL   string `json:"paypal_url"`
	CashAppURL  string `json:"cashapp_url"`
	CustomLabel string `json:"custom_label"`
	CustomURL   string `json:"custom_url"`
}

// Requests lets visitors search and request media through Seerr as one fixed Seerr user.
type Requests struct {
	Enabled bool          `json:"enabled"`
	URL     string        `json:"url"`
	APIKey  secret.String `json:"apiKey"`
	UserID  int64         `json:"userId"`
}

// Access lets visitors ask for a media server account; see the access package.
type Access struct {
	Provider        string        `json:"provider"`
	ServerPublicURL string        `json:"serverPublicUrl"`
	Enabled         bool          `json:"enabled"`
	PublicURL       string        `json:"publicUrl"`
	TemplateID      string        `json:"templateId"`
	SMTPHost        string        `json:"smtpHost"`
	SMTPPort        int           `json:"smtpPort"`
	SMTPSecurity    string        `json:"smtpSecurity"`
	SMTPUser        string        `json:"smtpUser"`
	SMTPPassword    secret.String `json:"smtpPassword"`
	From            string        `json:"from"`
	AdminEmail      string        `json:"adminEmail"`
}

type Feed struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalMinutes int    `json:"intervalMinutes"`
	Enabled         bool   `json:"enabled"`
}

// Rule picks items from feeds by title. Patterns are case-insensitive regular expressions.
type Rule struct {
	Name           string `json:"name"`
	Feed           string `json:"feed"` // feed name, or blank for all feeds
	MustContain    string `json:"mustContain"`
	MustNotContain string `json:"mustNotContain"`
	Category       string `json:"category"`
	Enabled        bool   `json:"enabled"`
}

// Watch is a folder whose .torrent, .nzb and .magnet files are sent through the
// same pipeline as everything else. Finished files go to the category's folder.
type Watch struct {
	Dir      string `json:"dir"`
	Category string `json:"category"` // blank = the category of the first RSS rule
	Enabled  bool   `json:"enabled"`
}

type Settings struct {
	Watches      []Watch    `json:"watches"`
	IntakePaused bool       `json:"intakePaused"` // new grabs are accepted but held until resumed
	Feeds        []Feed     `json:"feeds"`
	Rules        []Rule     `json:"rules"`
	Tdarr        Tdarr      `json:"tdarr"`
	Playback     Playback   `json:"playback"`
	Donations    Donations  `json:"donations"`
	Requests     Requests   `json:"requests"`
	Access       Access     `json:"access"`
	NZBGet       NZBGet     `json:"nzbget"`
	TorBox       TorBox     `json:"torbox"`
	Categories   []Category `json:"categories"`
	QBit         QBit       `json:"qbit"`

	// Login the arrs use against the fake client API.
	ClientUser     string        `json:"clientUser"`
	ClientPassword secret.String `json:"clientPassword"`

	// SkipQualityCheck turns off the check that a download's real video size matches the
	// quality the arr grabbed it as.
	SkipQualityCheck bool `json:"skipQualityCheck"`

	StallMinutes     int `json:"stallMinutes"`
	ReconcileSeconds int `json:"reconcileSeconds"`
	FlagImportHours  int `json:"flagImportHours"`

	// StorageScanMinutes is how often the library sizes are measured in the background.
	StorageScanMinutes int `json:"storageScanMinutes"`
}

func (s Settings) Category(name string) (Category, bool) {
	for _, c := range s.Categories {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Category{}, false
}

type Config struct {
	// Created is true on the first run, when the settings file did not exist yet.
	Created bool
	Env     Env
	mu      sync.RWMutex
	set     Settings
	path    string
}

func Load(env Env) (*Config, error) {
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return nil, err
	}
	if err := secret.LoadKey(filepath.Join(env.ConfigDir, "secret.key")); err != nil {
		return nil, err
	}
	c := &Config{Env: env, path: filepath.Join(env.ConfigDir, "settings.json")}
	b, err := os.ReadFile(c.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.set = Defaults(env)
		c.Created = true
		if err := c.save(); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &c.set); err != nil {
			return nil, err
		}
		fill(&c.set, env)
	}
	return c, nil
}

func Defaults(env Env) Settings {
	return Settings{
		Categories: []Category{
			{Name: "radarr", Kind: "radarr"},
			{Name: "tv-sonarr", Kind: "sonarr"},
			{Name: "lidarr", Kind: "lidarr"},
		},
		QBit:             QBit{SaveDir: filepath.Join(env.Work(), "client")},
		ClientUser:       "undermynd",
		ClientPassword:   secret.String(RandomToken(18)),
		StallMinutes:     60,
		ReconcileSeconds: 30,
		FlagImportHours:  6,
	}
}

func fill(s *Settings, env Env) {
	if s.Tdarr.InDir == "" {
		s.Tdarr.InDir = filepath.Join(env.Work(), "tdarr", "in")
	}
	if s.Tdarr.OutDir == "" {
		s.Tdarr.OutDir = filepath.Join(env.Work(), "tdarr", "out")
	}
	if s.Tdarr.MaxInFlight == 0 {
		s.Tdarr.MaxInFlight = 6
	}
	if s.Tdarr.StallMinutes == 0 {
		s.Tdarr.StallMinutes = 10
	}
	if s.Tdarr.TimeoutMinutes == 0 {
		s.Tdarr.TimeoutMinutes = 720
	}
	if s.TorBox.BaseURL == "" {
		s.TorBox.BaseURL = "https://api.torbox.app"
	}
	if s.TorBox.CreatePerHour == 0 {
		s.TorBox.CreatePerHour = 60
	}
	if s.TorBox.PullersAtOnce == 0 {
		s.TorBox.PullersAtOnce = 2
	}
	if s.TorBox.LinkGapSeconds == 0 {
		s.TorBox.LinkGapSeconds = 3
	}
	if s.StallMinutes == 0 {
		s.StallMinutes = 60
	}
	if s.ReconcileSeconds == 0 {
		s.ReconcileSeconds = 30
	}
	if s.FlagImportHours == 0 {
		s.FlagImportHours = 6
	}
	if s.StorageScanMinutes == 0 {
		s.StorageScanMinutes = 30
	}
	if s.QBit.SaveDir == "" {
		s.QBit.SaveDir = filepath.Join(env.Work(), "client")
	}
	if s.ClientUser == "" {
		s.ClientUser = "undermynd"
	}
}

func (c *Config) Get() Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.set
	s.Categories = append([]Category(nil), c.set.Categories...)
	return s
}

func (c *Config) Set(s Settings) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fill(&s, c.Env)
	old := c.set
	c.set = s
	if err := c.save(); err != nil {
		c.set = old
		return err
	}
	return nil
}

func (c *Config) save() error {
	b, err := json.MarshalIndent(c.set, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Folder layout under the shared data root.
func (e Env) StagingDir(id string) string {
	return filepath.Join(e.Work(), "staging", id)
}
func (e Env) CompleteRoot(category string) string {
	return filepath.Join(e.Complete(), SafeName(category))
}

func RandomToken(n int) string {
	const a = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = a[int(b[i])%len(a)]
	}
	return string(b)
}

// SafeName makes a single safe path element from a release name.
func SafeName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		if r < 32 {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(strings.TrimSpace(s), ".")
	if s == "" {
		s = "unnamed"
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Work is the folder for everything Overmynd creates while a job is in flight.
func (e Env) Work() string {
	if e.WorkDir != "" {
		return e.WorkDir
	}
	return filepath.Join(e.DataDir, "undermynd")
}

// Complete is the root of the folders the arrs import from.
func (e Env) Complete() string {
	if e.CompleteDir != "" {
		return e.CompleteDir
	}
	return filepath.Join(e.DataDir, "complete")
}
