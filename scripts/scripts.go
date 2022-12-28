package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func must(err error) {
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
}

// type MyFlag[T flag.Value] struct {
// 	Valid bool
// 	Value T
// }

// func (f *MyFlag[T]) Set(s string) error {
// 	err := f.Value.Set(s)
// 	if err != nil {
// 		return err
// 	}
// 	f.Valid = true
// 	return nil
// }

// func (f *MyFlag[T]) String() string {
// 	return f.Value.String()
// }

type SemanticVersion struct {
	major int
	minor int
	patch int
}

func ParseSemVer(s string) (SemanticVersion, error) {
	re := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

	res := re.FindAllStringSubmatch(s, -1)
	var sv SemanticVersion

	if len(res) == 0 || len(res[0]) < 4 {
		return SemanticVersion{}, fmt.Errorf("invalid semantic version: '%s'", s)
	}

	var err error
	sv.major, err = strconv.Atoi(res[0][1])
	if err != nil {
		return sv, err
	}
	sv.minor, err = strconv.Atoi(res[0][2])
	if err != nil {
		return sv, err
	}
	sv.patch, err = strconv.Atoi(res[0][3])
	if err != nil {
		return sv, err
	}

	return sv, nil
}

func (sv SemanticVersion) NextMajor() SemanticVersion {
	return SemanticVersion{
		major: sv.major + 1,
		minor: 0,
		patch: 0,
	}
}

func (sv SemanticVersion) NextMinor() SemanticVersion {
	return SemanticVersion{
		major: sv.major,
		minor: sv.minor + 1,
		patch: 0,
	}
}

func (sv SemanticVersion) NextPatch() SemanticVersion {
	return SemanticVersion{
		major: sv.major,
		minor: sv.minor,
		patch: sv.patch + 1,
	}
}

func (sv SemanticVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d", sv.major, sv.minor, sv.patch)
}

var (
	actionFlag  string
	releaseFlag string
	versionFlag string
)

func main() {

	flag.StringVar(&actionFlag, "action", "", "Choose your action")
	flag.StringVar(&releaseFlag, "release", "", "Creates a new release; options are patch, minor, and major")
	flag.StringVar(&versionFlag, "version", "", "Used with --release; specifies semver to bump (major, minor, patch) or an exact version (e.g. v1.2.3)")

	flag.Parse()

	switch actionFlag {
	case "":
		fmt.Println("An action is required")
		os.Exit(1)

	case "release":
		release()

	default:
		fmt.Printf("Invalid action: '%s'\n", actionFlag)
		os.Exit(1)
	}
}

func release() {
	fmt.Println("Cutting new release")

	gitDescribe, err := exec.Command("git", "describe", "--abbrev=0").Output()
	must(err)
	currentVersionStr := strings.TrimSpace(string(gitDescribe))
	fmt.Println("Current version:", currentVersionStr)

	currentVersion, err := ParseSemVer(currentVersionStr)
	must(err)

	var newVersion SemanticVersion
	switch versionFlag {
	case "":
		fmt.Println("--version is required with release")
		os.Exit(1)

	case "major":
		newVersion = currentVersion.NextMajor()
	case "minor":
		newVersion = currentVersion.NextMinor()
	case "patch":
		newVersion = currentVersion.NextPatch()
	default:
		newVersion, err = ParseSemVer(versionFlag)
		must(err)
	}

	fmt.Println("New version:", newVersion)

	err = exec.Command(
		"git", "tag",
		"-a", newVersion.String(),
		"-m", fmt.Sprintf("Version %s", newVersion.String()),
	).Run()
	must(err)
	fmt.Println("Tagged new version")
}
