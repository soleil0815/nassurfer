// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Dateien, in denen Chromium Passwörter und Formulardaten ablegt. Sie werden
// bei jedem Start gelöscht – falls je etwas darin gelandet ist.
var secretProfileFiles = []string{
	"Login Data", "Login Data-journal",
	"Login Data For Account", "Login Data For Account-journal",
	"Web Data", "Web Data-journal",
	"Affiliation Database", "Affiliation Database-journal",
}

// hardenProfile schaltet Passwort-Manager und Autofill im Profil fest ab.
// Anmelde-Cookies bleiben erhalten (die löscht "Anmeldungen und Verlauf löschen").
func hardenProfile(profile string) error {
	def := filepath.Join(profile, "Default")
	if err := os.MkdirAll(def, 0o700); err != nil {
		return err
	}
	for _, f := range secretProfileFiles {
		os.Remove(filepath.Join(def, f))
	}
	p := filepath.Join(def, "Preferences")
	prefs := map[string]any{}
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &prefs) // kaputte Datei: neu anlegen
	}
	sub := func(key string) map[string]any {
		m, ok := prefs[key].(map[string]any)
		if !ok {
			m = map[string]any{}
			prefs[key] = m
		}
		return m
	}
	prefs["credentials_enable_service"] = false
	prefs["credentials_enable_autosignin"] = false
	pr := sub("profile")
	pr["password_manager_enabled"] = false
	pr["password_manager_leak_detection"] = false
	af := sub("autofill")
	af["enabled"] = false
	af["profile_enabled"] = false
	af["credit_card_enabled"] = false
	pm := sub("password_manager")
	pm["autofillable_credentials_account_storage_opted_in"] = false
	b, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
