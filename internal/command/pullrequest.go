// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package command

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/googleapis/librarian/internal/githubrepo"
	"github.com/googleapis/librarian/internal/gitrepo"
)

// A PullRequestContent builds up the content of a pull request.
// Each entry in "Successes" represents a commit from a successful
// operation; each entry in "Errors" represents an unsuccessful
// operation which would otherwise have created a commit.
type PullRequestContent struct {
	Successes []string
	Errors    []string
}

// Add details to a PullRequestContent of a partial error which prevents a
// single API or library from being configured/regenerated/released,
// but without halting the overall process. A warning is logged locally with the error details,
// but we don't include detailed errors in the PR, as this could reveal sensitive information.
// The action should describe what failed, e.g. "configuring", "building", "generating".
func addErrorToPullRequest(pr *PullRequestContent, id string, err error, action string) {
	slog.Warn(fmt.Sprintf("Error while %s %s: %s", action, id, err))
	pr.Errors = append(pr.Errors, fmt.Sprintf("Error while %s %s", action, id))
}

// Adds a success entry to a PullRequestContent.
func addSuccessToPullRequest(pr *PullRequestContent, text string) {
	pr.Successes = append(pr.Successes, text)
}

// Creates a GitHub pull request based on the given content, with a title prefix (e.g. "feat: API regeneration")
// using a branch with a name of the form "librarian-{branchtype}-{timestamp}".
// If content is empty, the pull request is not created and no error is returned.
// If content only contains errors, the pull request is not created and an error is returned (to highlight that everything failed)
// If content contains any successes, a pull request is created and no error is returned (if the creation is successful) even if the content includes errors.
func createPullRequest(ctx *CommandContext, content *PullRequestContent, titlePrefix, descriptionSuffix, branchType string) (*githubrepo.PullRequestMetadata, error) {
	anySuccesses := len(content.Successes) > 0
	anyErrors := len(content.Errors) > 0

	var description string
	if !anySuccesses && !anyErrors {
		slog.Info("No PR to create, and no errors.")
		return nil, nil
	} else if !anySuccesses && anyErrors {
		slog.Error("No PR to create, but errors were logged (and restated below). Aborting.")
		for _, error := range content.Errors {
			slog.Error(error)
		}
		return nil, errors.New("errors encountered but no PR to create")
	} else if anySuccesses && !anyErrors {
		description = formatListAsMarkdown(content.Successes)
	} else {
		errorsText := formatListAsMarkdown(content.Errors)
		releasesText := formatListAsMarkdown(content.Successes)
		description = fmt.Sprintf("Errors:\n==================\n%s\n\nChanges Included:\n==================\n%s", errorsText, releasesText)
	}

	// Append any PR-specific additional information.
	if descriptionSuffix != "" {
		description = description + "\n" + descriptionSuffix
	}

	title := fmt.Sprintf("%s: %s", titlePrefix, formatTimestamp(ctx.startTime))

	if !flagPush {
		slog.Info(fmt.Sprintf("Push not specified; would have created PR with the following title and description:\n%s\n\n%s", title, description))
		return nil, nil
	}

	gitHubRepo, err := gitrepo.GetGitHubRepoFromRemote(ctx.languageRepo)
	if err != nil {
		return nil, err
	}

	branch := fmt.Sprintf("librarian-%s-%s", branchType, formatTimestamp(ctx.startTime))
	err = gitrepo.PushBranch(ctx.languageRepo, branch, githubrepo.GetAccessToken())
	if err != nil {
		slog.Info(fmt.Sprintf("Received error pushing branch: '%s'", err))
		return nil, err
	}
	return githubrepo.CreatePullRequest(ctx.ctx, gitHubRepo, branch, title, description)
}

// Formats the given list as a single Markdown string, with a "- " at the
// start of each value and a line break at the end of each value
func formatListAsMarkdown(list []string) string {
	var builder strings.Builder
	for _, value := range list {
		builder.WriteString(fmt.Sprintf("- %s\n", value))
	}
	return builder.String()
}
