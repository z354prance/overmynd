package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

func readTemplate(ctx context.Context, s Settings) (embyUser, error) {
	var template embyUser
	if err := emby(ctx, s, "GET", "/Users/"+s.TemplateID, nil, &template); err != nil {
		return template, errors.New("unable to read the Emby template user")
	}
	if template.ID != s.TemplateID || template.Policy == nil || template.Policy["IsDisabled"] != true || template.Policy["IsAdministrator"] != false {
		return template, errors.New("template must be a disabled, non-administrator Emby user")
	}
	if _, ok := template.Policy["EnableAllFolders"].(bool); !ok {
		return template, errors.New("template did not report its library access policy")
	}
	if template.Configuration == nil {
		return template, errors.New("template did not report its user configuration")
	}
	return template, nil
}

func copySettings(src map[string]any) map[string]any {
	// Both inputs originate from decoded JSON; copy nested arrays and objects too.
	b, _ := json.Marshal(src)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func templateSettings(template embyUser, disabled bool) (map[string]any, map[string]any) {
	policy := copySettings(template.Policy)
	policy["IsDisabled"] = disabled
	policy["IsAdministrator"] = false
	policy["IsHidden"] = true
	policy["IsHiddenRemotely"] = true
	// Lockout counters are account state, not template permissions.
	delete(policy, "InvalidLoginAttemptCount")
	delete(policy, "LockedOutDate")
	config := copySettings(template.Configuration)
	config["ProfilePin"] = "" // Do not give everyone the template's credential.
	return policy, config
}

func normalized(value any) any {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return v
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			item = normalized(item)
			if item != nil {
				out[key] = item
			}
		}
		return out
	case []any:
		if len(v) == 0 {
			return nil
		}
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalized(item)
		}
		return out
	default:
		return value
	}
}

func settingsDifference(expected, actual map[string]any, policy bool) string {
	if actual == nil {
		return "missing settings"
	}
	expected = copySettings(expected)
	actual = copySettings(actual)
	if policy {
		for _, key := range []string{"InvalidLoginAttemptCount", "LockedOutDate"} {
			delete(expected, key)
			delete(actual, key)
		}
	}
	if reflect.DeepEqual(normalized(expected), normalized(actual)) {
		return ""
	}
	keys := map[string]bool{}
	for key := range expected {
		keys[key] = true
	}
	for key := range actual {
		keys[key] = true
	}
	ordered := []string{}
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if !reflect.DeepEqual(normalized(expected[key]), normalized(actual[key])) {
			return key
		}
	}
	return "settings"
}

func verifySettings(ctx context.Context, s Settings, r Request, policy, config map[string]any) error {
	var saved embyUser
	if err := emby(ctx, s, "GET", "/Users/"+r.EmbyID, nil, &saved); err != nil {
		return errors.New("unable to read back Emby permissions")
	}
	if saved.ID != r.EmbyID || !strings.EqualFold(saved.Name, r.Username) {
		return errors.New("Emby account identity changed")
	}
	if field := settingsDifference(policy, saved.Policy, true); field != "" {
		return fmt.Errorf("Emby did not preserve template policy: %s", field)
	}
	if field := settingsDifference(config, saved.Configuration, false); field != "" {
		return fmt.Errorf("Emby did not preserve template configuration: %s", field)
	}
	return nil
}

// Use a separate bounded context: a cancelled request must still attempt to
// disable an account after a failed permission write or verification.
func quarantine(s Settings, r Request, baseline map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	policy := copySettings(baseline)
	if policy == nil {
		policy = map[string]any{}
	}
	for key := range policy {
		if strings.HasPrefix(key, "Enable") || strings.HasPrefix(key, "Allow") {
			if _, ok := policy[key].(bool); ok {
				policy[key] = false
			}
		}
	}
	policy["IsDisabled"] = true
	policy["IsAdministrator"] = false
	policy["EnableAllFolders"] = false
	policy["EnabledFolders"] = []string{}
	policy["EnableMediaPlayback"] = false
	policy["EnableRemoteAccess"] = false
	if err := emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Policy", policy, nil); err != nil {
		return errors.New("could not disable the Emby user; disable it manually in Emby")
	}
	var user embyUser
	if err := emby(ctx, s, "GET", "/Users/"+r.EmbyID, nil, &user); err != nil || user.Policy["IsDisabled"] != true || user.Policy["IsAdministrator"] != false {
		return errors.New("could not verify the user is disabled; disable it manually in Emby")
	}
	return nil
}

func applyTemplate(ctx context.Context, s Settings, r Request, template embyUser) (map[string]any, map[string]any, error) {
	policy, config := templateSettings(template, true)
	err := emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Policy", policy, nil)
	if err == nil {
		err = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Configuration", config, nil)
	}
	if err == nil {
		err = verifySettings(ctx, s, r, policy, config)
	}
	if err != nil {
		if disabledErr := quarantine(s, r, policy); disabledErr != nil {
			return nil, nil, fmt.Errorf("template could not be applied: %w", disabledErr)
		}
		return nil, nil, fmt.Errorf("template settings could not be verified; account remains disabled: %w", err)
	}
	return policy, config, nil
}

// ReapplyTemplate repairs a provisioned request only after an explicit admin
// action. Keep the account disabled and require a fresh recipient setup.
func (m *Manager) ReapplyTemplate(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !s.Enabled {
		return errors.New("access requests are disabled")
	}
	r, e := m.get(id)
	if e != nil {
		return errors.New("request not found")
	}
	if r.EmbyID == "" || (r.Status != "active" && r.Status != "awaiting_setup" && r.Status != "incomplete") {
		return errors.New("request has no provisioned account to repair")
	}
	var user embyUser
	if e = emby(ctx, s, "GET", "/Users/"+r.EmbyID, nil, &user); e != nil {
		return errors.New("unable to read the provisioned Emby user")
	}
	if user.ID != r.EmbyID || !strings.EqualFold(user.Name, r.Username) || user.Policy["IsAdministrator"] != false {
		return errors.New("Emby account identity or administrator permissions changed; inspect it manually")
	}
	if _, e = m.db.Exec("UPDATE access_requests SET status='incomplete',token_hash='',token_expires=0,error='Reapplying template',updated_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), id); e != nil {
		return e
	}
	if e = quarantine(s, r, user.Policy); e != nil {
		return m.fail(id, s, e.Error())
	}
	template, e := readTemplate(ctx, s)
	if e != nil {
		return m.fail(id, s, e.Error())
	}
	if _, _, e = applyTemplate(ctx, s, r, template); e != nil {
		return m.fail(id, s, e.Error())
	}
	if e = m.setState(id, "awaiting_setup", ""); e != nil {
		return e
	}
	return m.sendSetup(r, s)
}
