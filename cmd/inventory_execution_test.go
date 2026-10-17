package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Run the actual Cobra/config path in a fresh process: root/config globals and
// the caller cwd cannot contaminate other tests, including shuffled race runs.
func TestInventoryCommandSources(t *testing.T) {
	if raw := os.Getenv("PIG_TEST_INVENTORY_COMMAND"); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatal(err)
		}
		rootCmd.SetArgs(args)
		if err := rootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, family := range []string{"deploy", "do"} {
		for _, source := range []string{"absolute", "relative", "tilde", "environment", "config", "default", "ansible-config", "dynamic", "directory", "missing", "missing-env", "missing-config", "missing-default"} {
			t.Run(family+"/"+source, func(t *testing.T) {
				root := t.TempDir()
				root, _ = filepath.EvalSymlinks(root)
				home, caller, bin := filepath.Join(root, "home"), filepath.Join(root, "caller"), filepath.Join(root, "bin")
				pigsty := filepath.Join(home, "pigsty")
				capture := filepath.Join(root, "capture")
				for _, dir := range []string{pigsty, caller, bin, filepath.Join(home, ".pig")} {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				write := func(path, body string) {
					t.Helper()
					if err := os.WriteFile(path, []byte(body), 0700); err != nil {
						t.Fatal(err)
					}
				}
				write(filepath.Join(pigsty, "ansible.cfg"), "[defaults]\n")
				write(filepath.Join(pigsty, "deploy.yml"), "---\n")
				write(filepath.Join(pigsty, "pigsty.yml"), "all: {}\n")
				write(filepath.Join(home, ".pig", "config.yml"), "{}\n")
				write(filepath.Join(bin, "ansible-playbook"), "#!/bin/sh\npwd -P > \"$PIG_TEST_INVENTORY_CAPTURE\"\nprintf '%s\\n' \"$@\" >> \"$PIG_TEST_INVENTORY_CAPTURE\"\n")
				selected := filepath.Join(caller, "selected.yml")
				write(selected, "all: {}\n")
				args := []string{"--home", pigsty}
				envInventory := ""
				switch source {
				case "absolute":
					args = append(args, "-i", selected)
				case "relative":
					args = append(args, "-i", "selected.yml")
				case "tilde":
					selected = filepath.Join(home, "selected.yml")
					write(selected, "all: {}\n")
					args = append(args, "-i", "~/selected.yml")
				case "environment":
					envInventory = "selected.yml"
				case "config":
					selected = filepath.Join(pigsty, "selected.yml")
					write(selected, "all: {}\n")
					write(filepath.Join(home, ".pig", "config.yml"), "inventory: selected.yml\n")
				case "default":
					selected = filepath.Join(pigsty, "pigsty.yml")
				case "ansible-config":
					write(filepath.Join(pigsty, "ansible.cfg"), "[defaults]\ninventory = "+selected+"\n")
				case "dynamic":
					write(selected, "#!/bin/sh\nexit 99\n")
					args = append(args, "-i", selected)
				case "directory":
					selected = filepath.Join(caller, "inventory.d")
					if err := os.Mkdir(selected, 0700); err != nil {
						t.Fatal(err)
					}
					args = append(args, "-i", selected)
				case "missing":
					args = append(args, "-i", "absent.yml")
				case "missing-env":
					envInventory = "absent.yml"
				case "missing-config":
					write(filepath.Join(home, ".pig", "config.yml"), "inventory: absent.yml\n")
				case "missing-default":
					if err := os.Remove(filepath.Join(pigsty, "pigsty.yml")); err != nil {
						t.Fatal(err)
					}
				}
				// YAML fixtures are not dynamic sources; executable mode is only for the
				// deliberately dynamic fixture (Ansible, not PIG, owns its execution).
				if source != "dynamic" && source != "directory" {
					_ = os.Chmod(selected, 0600)
				}
				var tail []string
				if family == "deploy" {
					tail = []string{"deploy.yml"}
					args = append(args, "sty", "deploy")
				} else {
					tail = []string{"node.yml", "-l", "pg-test"}
					args = append(args, "do", "node-add", "pg-test")
				}
				raw, _ := json.Marshal(args)
				child := exec.Command(os.Args[0], "-test.run=^TestInventoryCommandSources$")
				child.Dir = caller
				child.Env = append(os.Environ(), "HOME="+home, "PIGSTY_HOME=", "PIGSTY_INVENTORY="+envInventory, "PIGSTY_CONFIG=", "ANSIBLE_INVENTORY=/wrong/default", "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "PIG_TEST_INVENTORY_COMMAND="+string(raw), "PIG_TEST_INVENTORY_CAPTURE="+capture)
				out, err := child.CombinedOutput()
				data, readErr := os.ReadFile(capture)
				reject := strings.HasPrefix(source, "missing") || (family == "deploy" && source == "directory")
				if reject {
					if err == nil || !os.IsNotExist(readErr) {
						t.Fatalf("invalid source executed: err=%v capture=%q output=%s", err, data, out)
					}
					return
				}
				if err != nil {
					t.Fatalf("command failed: %v\n%s", err, out)
				}
				resolvedHome, _ := filepath.EvalSymlinks(pigsty)
				want := append([]string{resolvedHome, "-i", selected}, tail...)
				got := strings.Split(strings.TrimSpace(string(data)), "\n")
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("actual cwd/argv=%q want=%q", got, want)
				}
			})
		}
	}
}
