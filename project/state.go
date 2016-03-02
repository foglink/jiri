// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"path/filepath"

	"v.io/jiri"
	"v.io/jiri/gitutil"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
)

type BranchState struct {
	HasGerritMessage bool
	Name             string
}

type ProjectState struct {
	Branches       []BranchState
	CurrentBranch  string
	HasUncommitted bool
	HasUntracked   bool
	Project        Project
}

func setProjectState(jirix *jiri.X, state *ProjectState, checkDirty bool, ch chan<- error) {
	var err error
	switch state.Project.Protocol {
	case "git":
		scm := gitutil.New(jirix.NewSeq(), gitutil.RootDirOpt(state.Project.Path))
		var branches []string
		branches, state.CurrentBranch, err = scm.GetBranches()
		if err != nil {
			ch <- err
			return
		}
		for _, branch := range branches {
			file := filepath.Join(state.Project.Path, jiri.ProjectMetaDir, branch, ".gerrit_commit_message")
			hasFile := true
			if _, err := jirix.NewSeq().Stat(file); err != nil {
				if !runutil.IsNotExist(err) {
					ch <- err
					return
				}
				hasFile = false
			}
			state.Branches = append(state.Branches, BranchState{
				Name:             branch,
				HasGerritMessage: hasFile,
			})
		}
		if checkDirty {
			state.HasUncommitted, err = scm.HasUncommittedChanges()
			if err != nil {
				ch <- err
				return
			}
			state.HasUntracked, err = scm.HasUntrackedFiles()
			if err != nil {
				ch <- err
				return
			}
		}
	default:
		ch <- UnsupportedProtocolErr(state.Project.Protocol)
		return
	}
	ch <- nil
}

func GetProjectStates(jirix *jiri.X, checkDirty bool) (map[ProjectKey]*ProjectState, error) {
	projects, err := LocalProjects(jirix, FastScan)
	if err != nil {
		return nil, err
	}
	states := make(map[ProjectKey]*ProjectState, len(projects))
	sem := make(chan error, len(projects))
	for key, project := range projects {
		state := &ProjectState{
			Project: project,
		}
		states[key] = state
		// jirix is not threadsafe, so we make a clone for each goroutine.
		go setProjectState(jirix.Clone(tool.ContextOpts{}), state, checkDirty, sem)
	}
	for _ = range projects {
		err := <-sem
		if err != nil {
			return nil, err
		}
	}
	return states, nil
}
