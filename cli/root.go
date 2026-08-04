// Package cli builds the asana-cli command tree and translates command
// results into process exit codes.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

// Exit codes (see spec): 0 success, 1 runtime/API/network error,
// 2 usage/config error.
const (
	exitOK      = 0
	exitRuntime = 1
	exitUsage   = 2
)

// usageError marks an error as a usage/config problem so Execute maps it to
// exit code 2 instead of the default runtime code 1.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// usageErrorf builds a usageError from a format string.
func usageErrorf(format string, args ...any) error {
	return &usageError{err: fmt.Errorf(format, args...)}
}

// globalOptions holds the persistent flags shared by every subcommand.
type globalOptions struct {
	human        bool
	verbose      bool
	timeout      time.Duration
	maxRetries   int
	retryMaxWait time.Duration
	noRetry      bool
}

// opts is populated by the root command's persistent flags before any
// subcommand RunE executes.
var opts globalOptions

// errNotImplemented is the placeholder returned by stub subcommands.
var errNotImplemented = errors.New("not implemented")

// version is the CLI version, overridden at build time via -ldflags
// "-X github.com/thaodangspace/asana-cli/cli.version=<value>" (see .goreleaser.yaml).
var version = "dev"

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "asana-cli",
		Short:         "CLI for Asana operations (JSON output by default)",
		Long:          "asana-cli exposes Asana operations as subcommands.\nOutput is JSON by default; pass --human for readable summaries.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Flag-parse failures are usage errors (exit code 2).
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{err: err}
	})

	pf := root.PersistentFlags()
	pf.BoolVar(&opts.human, "human", false, "print human-readable summaries instead of JSON")
	pf.BoolVar(&opts.verbose, "verbose", false, "log request method and path to stderr (never the token)")
	pf.DurationVar(&opts.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	pf.IntVar(&opts.maxRetries, "max-retries", asana.DefaultMaxRetries, "maximum retries for transient GET/DELETE requests")
	pf.DurationVar(&opts.retryMaxWait, "retry-max-wait", asana.DefaultRetryMaxWait, "maximum wait for a single retry")
	pf.BoolVar(&opts.noRetry, "no-retry", false, "disable retries and make one request only")

	root.AddCommand(
		newMeCommand(),
		newListWorkspacesCommand(),
		newListProjectsCommand(),
		newGetProjectCommand(),
		newCreateProjectCommand(),
		newUpdateProjectCommand(),
		newDeleteProjectCommand(),
		newDuplicateProjectCommand(),
		newSearchProjectsCommand(),
		newListTeamProjectsCommand(),
		newListProjectTasksCommand(),
		newListTagTasksCommand(),
		newSearchTasksCommand(),
		newGetTaskCommand(),
		newListTaskStoriesCommand(),
		newListTaskAttachmentsCommand(),
		newGetAttachmentCommand(),
		newDownloadAttachmentCommand(),
		newAddAttachmentCommand(),
		newCommentOnTaskCommand(),
		newCreateTaskCommand(),
		newUpdateTaskCommand(),
		newDeleteTaskCommand(),
		newDuplicateTaskCommand(),
		newListSectionsCommand(),
		newGetSectionCommand(),
		newCreateSectionCommand(),
		newUpdateSectionCommand(),
		newDeleteSectionCommand(),
		newMoveSectionCommand(),
		newAddTaskToSectionCommand(),
		newListSectionTasksCommand(),
		newListSubtasksCommand(),
		newCreateSubtaskCommand(),
		newSetTaskParentCommand(),
		newRemoveTaskParentCommand(),
		newListDependenciesCommand(),
		newListDependentsCommand(),
		newAddDependencyCommand(),
		newRemoveDependencyCommand(),
		newAddTaskToProjectCommand(),
		newRemoveTaskFromProjectCommand(),
		newAddTagToTaskCommand(),
		newRemoveTagFromTaskCommand(),
		newAddTaskFollowersCommand(),
		newRemoveTaskFollowersCommand(),
		newAPICommand(),
	)

	return root
}

// Execute runs the root command and returns the process exit code.
func Execute() int {
	root := newRootCommand()
	err := root.Execute()
	if err == nil {
		return exitOK
	}
	writeError(os.Stderr, err, opts.human)
	return exitCodeFor(err)
}

// exitCodeFor maps an error to a process exit code: usage/config errors → 2,
// everything else → 1.
func exitCodeFor(err error) int {
	if err == nil {
		return exitOK
	}
	var ue *usageError
	if errors.As(err, &ue) {
		return exitUsage
	}
	// Cobra reports unknown commands/flags as plain errors; treat as usage.
	if msg := err.Error(); strings.HasPrefix(msg, "unknown command") ||
		strings.HasPrefix(msg, "unknown flag") ||
		strings.HasPrefix(msg, "unknown shorthand flag") {
		return exitUsage
	}
	return exitRuntime
}
