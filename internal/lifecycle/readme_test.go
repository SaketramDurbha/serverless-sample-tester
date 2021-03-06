package lifecycle

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// setEnv takes a map of environment variables to their values and sets the program's environment accordingly.
func setEnv(e map[string]string) error {
	for k, v := range e {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// unsetEnv takes a map of environment variables to their values and unsets the environment variables in the program's
// environment.
func unsetEnv(e map[string]string) error {
	for k := range e {
		if err := os.Unsetenv(k); err != nil {
			return err
		}
	}
	return nil
}

// uniqueServiceName is the Cloud Run Service name that will replace the existing service names in each codeBlock test.
const uniqueServiceName = "unique_service_name"

// uniqueGCRURL is the Container Registry URL tag that will replace the existing Container Registry URL tag in each codeBlock test.
const uniqueGCRURL = "gcr.io/unique/tag"

type toCommandsTest struct {
	codeBlock codeBlock         // input code block
	cmds      []*exec.Cmd       // expected result of codeBlock.toCommands
	err       string            // expected string contained in return error of codeBlock.toCommands
	env       map[string]string // map of environment variables to values for this test
}

var toCommandsTests = []toCommandsTest{
	// single one-line command
	{
		codeBlock: codeBlock{
			"echo hello world",
		},
		cmds: []*exec.Cmd{
			exec.Command("echo", "hello", "world"),
		},
	},

	// two one-line commands
	{
		codeBlock: codeBlock{
			"echo line one",
			"echo line two",
		},
		cmds: []*exec.Cmd{
			exec.Command("echo", "line", "one"),
			exec.Command("echo", "line", "two"),
		},
	},

	// single multiline command
	{
		codeBlock: codeBlock{
			"echo multi \\",
			"line command",
		},
		cmds: []*exec.Cmd{
			exec.Command("echo", "multi", "line", "command"),
		},
	},

	// line cont char but code block closes at next line
	{
		codeBlock: codeBlock{
			"echo multi \\",
		},
		cmds: nil,
		err:  errCodeBlockEndAfterLineCont,
	},

	// expand environment variable test
	{
		codeBlock: codeBlock{
			"echo ${TEST_ENV}",
		},
		cmds: []*exec.Cmd{
			exec.Command("echo", "hello", "world"),
		},
		env: map[string]string{
			"TEST_ENV": "hello world",
		},
	},

	// replace Cloud Run service name with provided name test
	{
		codeBlock: codeBlock{
			"gcloud run services deploy hello_world",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "run", "services", "deploy", uniqueServiceName),
		},
	},

	// replace Container Registry URL with provided URL test
	{
		codeBlock: codeBlock{
			"gcloud builds submit --tag=gcr.io/hello/world",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "builds", "submit", "--tag="+uniqueGCRURL),
		},
	},

	// replace multiline GCR URL with provided URL test
	{
		codeBlock: codeBlock{
			"gcloud builds submit --tag=gcr.io/hello/\\",
			"world",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "builds", "submit", "--tag="+uniqueGCRURL),
		},
	},

	// replace Cloud Run service name and GCR URL with provided inputs test
	{
		codeBlock: codeBlock{
			"gcloud run services deploy hello_world --image=gcr.io/hello/world",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "run", "services", "deploy", uniqueServiceName, "--image="+uniqueGCRURL),
		},
	},

	// replace Cloud Run service name and GCR URL with `--image url` syntax test
	// this test breaks right now (issue #3)
	//{
	//	codeBlock: codeBlock{
	//		"gcloud run services deploy hello_world --image gcr.io/hello/world",
	//	},
	//	cmds: []*exec.Cmd{
	//		exec.Command("gcloud", "--quiet", "run", "services", "deploy", uniqueServiceName, "--image", uniqueGCRURL),
	//	},
	//	serviceName: "unique_service_name",
	//	gcrURL: "gcr.io/unique/tag",
	//},
	{
		codeBlock: codeBlock{
			"gcloud run services deploy hello_world --image=gcr.io/hello/world --add-cloudsql-instances=${TEST_CLOUD_SQL_CONNECTION}",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "run", "services", "deploy", uniqueServiceName, "--image="+uniqueGCRURL, "--add-cloudsql-instances=project:region:instance"),
		},
		env: map[string]string{
			"TEST_CLOUD_SQL_CONNECTION": "project:region:instance",
		},
	},

	// replace Cloud Run service name provided name in command with multiline arguments test
	{
		codeBlock: codeBlock{
			"gcloud run services update hello_world --add-cloudsql-instances=\\",
			"project:region:instance",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "run", "services", "update", uniqueServiceName, "--add-cloudsql-instances=project:region:instance"),
		},
	},

	// replace Cloud Run service name provided name and expand environment variables in command with multiline arguments test
	{
		codeBlock: codeBlock{
			"gcloud run services update hello_world --add-cloudsql-instances=\\",
			"${TEST_CLOUD_SQL_CONNECTION}",
		},
		cmds: []*exec.Cmd{
			exec.Command("gcloud", "--quiet", "run", "services", "update", uniqueServiceName, "--add-cloudsql-instances=project:region:instance"),
		},
		env: map[string]string{
			"TEST_CLOUD_SQL_CONNECTION": "project:region:instance",
		},
	},
}

func TestToCommands(t *testing.T) {
	for i, tc := range toCommandsTests {
		if len(tc.codeBlock) == 0 {
			continue
		}

		if err := setEnv(tc.env); err != nil {
			t.Errorf("#%d: setEnv: %v", i, err)

			if err = unsetEnv(tc.env); err != nil {
				t.Errorf("#%d: unsetEnv: %v", i, err)
			}

			continue
		}

		cmds, err := tc.codeBlock.toCommands(uniqueServiceName, uniqueGCRURL)

		var errorMatch bool
		if err == nil {
			errorMatch = tc.err == ""
		} else {
			errorMatch = strings.Contains(err.Error(), tc.err)
		}

		if !errorMatch {
			t.Errorf("#%d: error mismatch\nwant: %s\ngot: %v", i, tc.err, err)
		}

		if (errorMatch && err == nil) && !reflect.DeepEqual(cmds, tc.cmds) {
			t.Errorf("#%d: result mismatch\nwant: %#+v\ngot: %#+v", i, tc.cmds, cmds)
		}

		if err := unsetEnv(tc.env); err != nil {
			t.Errorf("#%d: unsetEnv: %v", i, err)
		}
	}
}

type parseREADMETest struct {
	inFileName string    // input Markdown file
	lifecycle  Lifecycle // expected result of parseREADME
	err        error     // expected parseREADME return error
}

var parseREADMETests = []parseREADMETest{
	// three code blocks, only two with comment code tags. one with one command, the other with two commands
	{
		inFileName: "readme_test.md",
		lifecycle: Lifecycle{
			exec.Command("echo", "hello", "world"),
			exec.Command("echo", "line", "one"),
			exec.Command("echo", "line", "two"),
		},
	},
}

func TestParseREADME(t *testing.T) {
	for i, tc := range parseREADMETests {
		if tc.inFileName == "" {
			continue
		}

		// Cloud Run Service name and Container Registry URL tag replacement will be tested in TestToCommands
		lifecycle, err := parseREADME(tc.inFileName, "", "")

		if !errors.Is(err, tc.err) {
			t.Errorf("#%d: error mismatch\nwant: %v\ngot: %v", i, tc.err, err)
			continue
		}

		if err == nil && !reflect.DeepEqual(lifecycle, tc.lifecycle) {
			t.Errorf("#%d: result mismatch\nwant: %#+v\ngot: %#+v", i, tc.lifecycle, lifecycle)
			continue
		}
	}
}

type extractLifecycleTest struct {
	in        string    // input Markdown string
	lifecycle Lifecycle // expected results of extractLifecycle on in
	err       error     // expected error
}

var extractLifecycleTests = []extractLifecycleTest{
	// single code block
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo hello world\n" +
			"```\n",
		lifecycle: Lifecycle{
			exec.Command("echo", "hello", "world"),
		},
	},

	// two code blocks with markdown text in the middle
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo build command\n" +
			"```\n" +
			"markdown instructions\n" +
			"[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo deploy command\n" +
			"```\n",
		lifecycle: Lifecycle{
			exec.Command("echo", "build", "command"),
			exec.Command("echo", "deploy", "command"),
		},
	},
}

func TestExtractLifecycle(t *testing.T) {
	for i, tc := range extractLifecycleTests {
		if tc.in == "" {
			continue
		}

		s := bufio.NewScanner(strings.NewReader(tc.in))

		// Cloud Run Service name and Container Registry URL tag replacement will be tested in TestToCommands
		lifecycle, err := extractLifecycle(s, "", "")

		if !errors.Is(err, tc.err) {
			t.Errorf("#%d: error mismatch\nwant: %v\ngot: %v", i, tc.err, err)
			continue
		}

		if err == nil && !reflect.DeepEqual(lifecycle, tc.lifecycle) {
			t.Errorf("#%d: result mismatch\nwant: %#+v\ngot: %#+v", i, tc.lifecycle, lifecycle)
		}
	}
}

type extractCodeBlocksTest struct {
	in         string      // input Markdown string
	codeBlocks []codeBlock // expected result of extractCodeBlocks
	err        error       // expected return error of extractCodeBlocks
}

var extractCodeBlocksTests = []extractCodeBlocksTest{
	// single code block
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo hello world\n" +
			"```\n",
		codeBlocks: []codeBlock{
			[]string{
				"echo hello world",
			},
		},
	},

	// code block not closed
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo hello world\n",
		codeBlocks: nil,
		err:        errCodeBlockNotClosed,
	},

	// code block doesn't start immediately after code tag
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"not start of code block\n" +
			"```\n" +
			"echo hello world\n" +
			"```\n",
		codeBlocks: nil,
		err:        errCodeBlockStartNotFound,
	},

	// EOF immediately after code tag
	{
		in: "instuctions\n" +
			"[//]: # ({sst-run-unix})\n",
		codeBlocks: nil,
		err:        errEOFAfterCodeTag,
	},

	// single code block, two lines
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo line one\n" +
			"echo line two\n" +
			"```\n",
		codeBlocks: []codeBlock{
			[]string{
				"echo line one",
				"echo line two",
			},
		},
	},

	// two code blocks with markdown instructions in the middle
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo build command\n" +
			"```\n" +
			"markdown instructions\n" +
			"[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo deploy command\n" +
			"```\n",
		codeBlocks: []codeBlock{
			[]string{
				"echo build command",
			},
			[]string{
				"echo deploy command",
			},
		},
	},

	// two code blocks, but only one is annotated with code tag
	{
		in: "[//]: # ({sst-run-unix})\n" +
			"```\n" +
			"echo build and deploy command\n" +
			"```\n" +
			"markdown instructions\n" +
			"```\n" +
			"echo irrelevant command\n" +
			"```\n",
		codeBlocks: []codeBlock{
			[]string{
				"echo build and deploy command",
			},
		},
	},

	// one code block, but not annotated with code tag
	{
		in: "```\n" +
			"echo hello world\n" +
			"```\n",
		codeBlocks: nil,
	},
}

func TestExtractCodeBlocks(t *testing.T) {
	for i, tc := range extractCodeBlocksTests {
		if tc.in == "" {
			continue
		}

		s := bufio.NewScanner(strings.NewReader(tc.in))
		codeBlocks, err := extractCodeBlocks(s)

		if !errors.Is(err, tc.err) {
			t.Errorf("#%d: error mismatch\nwant: %v\ngot: %v", i, tc.err, err)
			continue
		}

		if err == nil && !reflect.DeepEqual(codeBlocks, tc.codeBlocks) {
			t.Errorf("#%d: result mismatch\nwant: %#+v\ngot: %#+v", i, tc.codeBlocks, codeBlocks)
		}
	}
}
