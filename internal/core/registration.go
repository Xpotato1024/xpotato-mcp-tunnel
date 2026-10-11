package core

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
)

func validateEndpoint(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("--mcp-url must be an owner-verified HTTPS MCP endpoint without credentials/query/fragment")
	}
	return nil
}

// Tunnel integration is a product-specific UI operation. Do not invent a
// streamable HTTP endpoint from a Tunnel ID or claim account registration.
func validateRegistration(d Device) error {
	if d.MCPURL == "" {
		return nil
	}
	if e := validateEndpoint(d.MCPURL); e != nil {
		return e
	}
	if len("xpotato-"+d.Name) > 64 {
		return errors.New("plugin bundle name exceeds 64 characters; use a shorter device name")
	}
	return nil
}

func Registration(o Options, d Device) error {
	if e := validateRegistration(d); e != nil {
		return e
	}
	root := filepath.Join(directory(o), "registration")
	if e := atomicJSON(filepath.Join(root, "tunnel-registration.json"), map[string]any{"schemaVersion": 1, "device": d.Name, "tunnelId": d.Tunnel, "status": "account-registration-not-probed", "url": "https://chatgpt.com/plugins", "steps": []string{"Reuse the existing plugin bound to this same Tunnel ID; do not create a duplicate", "If no plugin exists: start this device's tunnel, add custom MCP server, choose Tunnel and this ID", "For new registration only: review authentication and permissions, then create as a plugin"}}); e != nil {
		return e
	}
	if d.MCPURL == "" {
		return nil
	}
	name := "xpotato-" + d.Name
	plugin := map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "name": name, "version": Version, "description": "Owner-controlled Desktop Commander device", "author": map[string]string{"name": "Xpotato"}, "extensions": map[string]any{"com.openai": map[string]any{"interface": map[string]string{"displayName": "Xpotato " + d.Name, "shortDescription": "Desktop Commander device", "developerName": "Xpotato", "category": "Productivity", "defaultPrompt": "Use my approved device directories."}}}}
	mcp := map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": map[string]any{d.Name: map[string]string{"type": "streamable-http", "url": d.MCPURL}}}
	f, e := os.CreateTemp(root, ".bundle-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	for _, entry := range []struct {
		Name  string
		Value any
	}{{"plugin.json", plugin}, {"mcp.json", mcp}} {
		b, err := json.MarshalIndent(entry.Value, "", "  ")
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
		w, err := z.Create(name + "/" + entry.Name)
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
		if _, err = w.Write(append(b, '\n')); err != nil {
			z.Close()
			f.Close()
			return err
		}
	}
	if e = z.Close(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(root, name+".zip"))
}
