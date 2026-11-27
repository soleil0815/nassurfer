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

// Verlauf, Sitzungen, Vorschläge und Zwischenspeicher: NASsurfer ist ein
// Notfall-Browser und soll nichts davon behalten. Gelöscht bei jedem Start und
// jedes Mal, wenn Chromium endet. Favoriten liegen außerhalb des Profils;
// Anmelde-Cookies bleiben, damit man angemeldet bleibt.
var historyProfileFiles = []string{
	"History", "History-journal",
	"Visited Links",
	"Top Sites", "Top Sites-journal",
	"Shortcuts", "Shortcuts-journal",
	"Network Action Predictor", "Network Action Predictor-journal",
	"Favicons", "Favicons-journal",
	"DIPS", "DIPS-journal",
	"Current Session", "Current Tabs", "Last Session", "Last Tabs",
	"Sessions", "Session Storage",
	"Cache", "Code Cache", "GPUCache", "DawnGraphiteCache", "DawnWebGPUCache",
}

// clearHistory entfernt alles, woraus sich ablesen lässt, welche Seiten
// besucht wurden.
func clearHistory(profile string) {
	def := filepath.Join(profile, "Default")
	for _, f := range historyProfileFiles {
		os.RemoveAll(filepath.Join(def, f))
	}
}

// hardenProfile schaltet Passwort-Manager, Autofill und Verlauf im Profil fest ab.
// Anmelde-Cookies bleiben erhalten (die löscht "Anmeldungen und Verlauf löschen").
func hardenProfile(profile string) error {
	def := filepath.Join(profile, "Default")
	if err := os.MkdirAll(def, 0o700); err != nil {
		return err
	}
	for _, f := range secretProfileFiles {
		os.Remove(filepath.Join(def, f))
	}
	clearHistory(profile)
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
	hi := sub("history")
	hi["saving_disabled"] = true // kein Verlauf
	hi["deleting_enabled"] = true
	se := sub("search")
	se["suggest_enabled"] = false
	sess := sub("session")
	sess["restore_on_startup"] = 5 // immer leer starten, keine alten Tabs
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
