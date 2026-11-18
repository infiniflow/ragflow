//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/cli/filesystem/skill_hub/source"
)

func TestInstallSkillRejectsUnsafeNameBeforeCreatingFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skill-install-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	t.Setenv("TMPDIR", tmpDir)

	command := &SkillInstallCommand{}
	bundle := &source.SkillBundle{Name: "../outside", Files: map[string][]byte{"SKILL.md": []byte("content")}}
	if err := command.installSkill(context.Background(), "default", bundle, false); err == nil {
		t.Fatal("installSkill accepted a path-traversal skill name")
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("installSkill created files before rejecting the name: %v", entries)
	}
}

func TestStageSkillBundleRejectsInvalidMetadataBeforeReturning(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	bundle := &source.SkillBundle{
		Name:     "sample-skill",
		Metadata: &source.SkillMetadata{Version: "banana"},
		Files: map[string][]byte{
			"SKILL.md": []byte("---\nname: sample-skill\ndescription: test skill\n---\ncontent\n"),
		},
	}
	if _, _, err := stageSkillBundle(bundle); err == nil {
		t.Fatal("stageSkillBundle accepted an invalid skill version")
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid skill left staged files behind: %v", entries)
	}
}

func TestResolveSkillFilePathRejectsTraversal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skill")
	for _, relPath := range []string{"../outside", "../../tmp/outside", "/tmp/outside", `..\outside`, ""} {
		t.Run(relPath, func(t *testing.T) {
			if _, err := resolveSkillFilePath(root, relPath); err == nil {
				t.Fatalf("resolveSkillFilePath(%q) succeeded", relPath)
			}
		})
	}
}

func TestResolveSkillFilePathAllowsNestedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skill")
	got, err := resolveSkillFilePath(root, "references/guide.md")
	if err != nil {
		t.Fatalf("resolveSkillFilePath: %v", err)
	}
	if want := filepath.Join(root, "references", "guide.md"); got != want {
		t.Fatalf("resolveSkillFilePath = %q, want %q", got, want)
	}
}

func TestResolveSkillFilePathNormalizesWindowsSeparators(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skill")
	got, err := resolveSkillFilePath(root, `references\guide.md`)
	if err != nil {
		t.Fatalf("resolveSkillFilePath: %v", err)
	}
	if want := filepath.Join(root, "references", "guide.md"); got != want {
		t.Fatalf("resolveSkillFilePath = %q, want %q", got, want)
	}
}
