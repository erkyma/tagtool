package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
)

const usage = `usage: tt [scope] [stage] [flags]

Creates the next semver git tag. You pick the kind of release; tt works out
the number from existing tags and tags HEAD after asking you to confirm.

stage (what kind of release)
  a,  alpha     early pre-release            6.7.0-alpha.1
  b,  beta      pre-release                  6.7.0-beta.3
  rc            release candidate            6.7.0-rc.1
  f,  final     closes the open line         6.7.0

scope (which number to bump, from the latest final)
  ma, major     breaking change              6.6.1 -> 7.0.0
  mi, minor     new feature                  6.6.1 -> 6.7.0
  p,  patch     bugfix                       6.6.1 -> 6.6.2

  Without a scope, a stage continues the open pre-release line, or starts a
  new minor line if the latest tag is a final. Without a stage, a scope tags
  a final directly.

flags
  -l, --list [N]   list the N highest tags with their commits (default 5)
  -r, --at <ref>   tag <ref> (commit, branch) instead of HEAD
  -p, --push       push the new tag to origin
  -n, --dry-run    show what would be tagged, change nothing
  -y, --yes        don't ask for confirmation (finals always ask)
  -o, --offline    skip 'git fetch --tags' before resolving
  -v, --version    print the version
  -h, --help       show this help

safety
  Never goes back: no beta after an rc, no pre-release after a final on the
  same version. Refuses dirty working trees and already-tagged commits.

examples (latest final 6.6.1, open line 6.7.0-beta.2)
  tt               show status, offer 6.7.0-beta.3
  tt b             6.7.0-beta.3   next beta on the open line
  tt rc            6.7.0-rc.1     promote the open line to release candidate
  tt f             6.7.0          release the open line
  tt p             6.6.2          hotfix on the latest final
  tt p b           6.6.2-beta.1   beta of a hotfix
  tt ma b          7.0.0-beta.1   start a new major line
  tt b -n          preview the next beta without tagging
  tt b -p          tag the next beta and push it
  tt f -r abc123   release a specific commit
  tt -l 10         list the 10 highest tags

shell completion (zsh)
  source <(tt completion zsh)`

const zshCompletion = `_tt() {
  _arguments \
    '(-l --list)'{-l,--list}'[list the highest tags]::count:' \
    '(-r --at)'{-r,--at}'[tag ref instead of HEAD]:ref:' \
    '(-p --push)'{-p,--push}'[push the tag to origin]' \
    '(-n --dry-run)'{-n,--dry-run}'[show what would be tagged]' \
    '(-y --yes)'{-y,--yes}'[skip confirmation]' \
    '(-o --offline)'{-o,--offline}'[do not fetch tags first]' \
    '(-v --version)'{-v,--version}'[print the version]' \
    '(-h --help)'{-h,--help}'[show help]' \
    '*:command:((ma\:major mi\:minor p\:patch a\:alpha b\:beta rc\:release\ candidate f\:final))'
}
compdef _tt tt tagtool`

var (
	scopes = map[string]string{"ma": "major", "major": "major", "mi": "minor", "minor": "minor", "p": "patch", "patch": "patch"}
	stages = map[string]string{"a": "alpha", "alpha": "alpha", "b": "beta", "beta": "beta", "rc": "rc", "f": "final", "final": "final"}
)

type opts struct {
	scope, stage, ref       string
	list                    int
	push, dry, yes, offline bool
}

func main() {

	if len(os.Args) == 3 && os.Args[1] == "completion" && os.Args[2] == "zsh" {
		fmt.Println(zshCompletion)
		return
	}

	o, err := parseArgs(os.Args[1:])
	if err != nil {
		die(err.Error() + " (see tt -h)")
	}

	if _, err := git("rev-parse", "--git-dir"); err != nil {
		die("not a git repository")
	}

	if !o.offline {
		if remotes, _ := git("remote"); remotes != "" {
			if _, err := git("fetch", "--tags", "--quiet"); err != nil {
				die(fmt.Sprintf("fetching tags failed (use -o to skip): %v", err))
			}
		}
	}

	tags := versionTags()
	if o.list > 0 {
		list(tags, o.list)
		return
	}

	run(o, tags)
}

func run(o opts, tags []Version) {

	ref := o.ref
	if ref == "" {
		ref = "HEAD"
	}
	sha, err := git("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		die("unknown ref: " + ref)
	}
	commit, _ := git("log", "-1", "--format=%h \"%s\"", sha)

	latest := "none"
	if len(tags) > 0 {
		latest = sortDesc(tags)[0].String()
	}

	if o.scope == "" && o.stage == "" {
		status(tags, commit)
	}

	next, err := Next(tags, o.scope, o.stage)
	if err != nil {
		die(err.Error())
	}

	if existing, _ := git("tag", "--points-at", sha); existing != "" {
		for _, t := range strings.Fields(existing) {
			if _, ok := Parse(t); ok {
				die(fmt.Sprintf("%s is already tagged %s", commit, t))
			}
		}
	}

	if o.ref == "" {
		if dirty, _ := git("status", "--porcelain", "--untracked-files=no"); dirty != "" {
			die("working tree has uncommitted changes; commit them or tag a specific ref with -r")
		}
	}

	fmt.Printf("%s → %s  @ %s\n", latest, next, commit)
	if o.dry {
		return
	}

	final := next.Stage == ""
	if (!o.yes || final) && !confirm(final) {
		fmt.Println("aborted")
		return
	}

	if _, err := git("tag", "-a", next.String(), "-m", next.String(), sha); err != nil {
		die(err.Error())
	}
	fmt.Println("tagged", next)

	if o.push {
		// ponytail: assumes the remote is called origin; add a flag if anyone uses another name
		if _, err := git("push", "origin", "refs/tags/"+next.String()); err != nil {
			die(err.Error())
		}
		fmt.Println("pushed", next)
	}
}

func parseArgs(args []string) (opts, error) {

	o := opts{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-l", "--list":
			o.list = 5
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					o.list = n
					i++
				}
			}
		case "-r", "--at":
			if i+1 >= len(args) {
				return o, errors.New(a + " needs a ref")
			}
			i++
			o.ref = args[i]
		case "-p", "--push":
			o.push = true
		case "-n", "--dry-run":
			o.dry = true
		case "-y", "--yes":
			o.yes = true
		case "-o", "--offline":
			o.offline = true
		case "-v", "--version":
			fmt.Println("tagtool", version())
			os.Exit(0)
		case "-h", "--help":
			fmt.Println(usage)
			os.Exit(0)
		default:
			if s, ok := scopes[a]; ok && o.scope == "" {
				o.scope = s
			} else if s, ok := stages[a]; ok && o.stage == "" {
				o.stage = s
			} else {
				return o, fmt.Errorf("unexpected argument %q", a)
			}
		}
	}

	return o, nil
}

func status(tags []Version, commit string) {

	final, open := "-", "-"
	for _, t := range sortDesc(tags) {
		if t.Stage == "" {
			final = t.String()
			break
		}
		if open == "-" {
			open = t.String()
		}
	}

	fmt.Printf("latest final   %s\nopen line      %s\nHEAD           %s\n\n", final, open, commit)
}

func list(tags []Version, n int) {

	for i, t := range sortDesc(tags) {
		if i == n {
			break
		}
		sha, _ := git("rev-parse", "--short", t.String()+"^{commit}")
		fmt.Printf("%-20s %s\n", t, sha)
	}
}

func versionTags() []Version {

	out, err := git("tag", "-l")
	if err != nil {
		die(err.Error())
	}

	var tags []Version
	for _, t := range strings.Fields(out) {
		if v, ok := Parse(t); ok {
			tags = append(tags, v)
		}
	}

	return tags
}

func confirm(final bool) bool {

	prompt := "[y/N] "
	if final {
		prompt = "this is a FINAL release. [y/N] "
	}
	fmt.Print(prompt)

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))

	return answer == "y" || answer == "yes"
}

func git(args ...string) (string, error) {

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)))
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func die(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, "tt:", msg)
	os.Exit(1)
}

// version is stamped by the Go toolchain: the tag for go install ...@v1.2.3 or a clean tagged
// checkout, otherwise a pseudo-version with the commit (and +dirty for local changes).
func version() string {

	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}

	return "unknown"
}
