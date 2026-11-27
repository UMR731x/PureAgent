package update

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const HelperFlag = "--pureagent-install-update"
const RestartFlag = "--pureagent-restarted"

type installPlan struct {
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
	ParentPID        int      `json:"parent_pid"`
	HelperPID        int      `json:"helper_pid"`
	Failure          string   `json:"failure,omitempty"`
}

type installation struct {
	directory string
	plan      installPlan
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func (s *installation) path(name string) string { return filepath.Join(s.directory, name) }

func (s *installation) savePlan() error {
	encoded, err := json.Marshal(s.plan)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path("plan.json"), encoded, 0600)
}

func readInstallation(path string) (*installation, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &installation{directory: filepath.Dir(path)}
	if err := json.Unmarshal(encoded, &s.plan); err != nil {
		return nil, err
	}
	return s, nil
}

func prepare(ctx context.Context, client *http.Client, asset releaseAsset, arguments []string) (*installation, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(filepath.Dir(executable), ".pureagent-update-")
	if err != nil {
		return nil, fmt.Errorf("无法在客户端目录准备更新：%w", err)
	}
	s := &installation{directory: directory, plan: installPlan{
		Executable: executable, Arguments: arguments, WorkingDirectory: workingDirectory, ParentPID: os.Getpid(),
	}}
	if err := s.stageFiles(ctx, client, asset); err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	return s, nil
}

func (s *installation) stageFiles(ctx context.Context, client *http.Client, asset releaseAsset) error {
	original, err := os.Open(s.plan.Executable)
	if err != nil {
		return err
	}
	defer original.Close()
	stat, err := original.Stat()
	if err != nil {
		return err
	}
	if err := download(ctx, client, asset, s.path(executableName("next")), stat.Mode()); err != nil {
		return err
	}
	helper, err := os.OpenFile(s.path(executableName("installer")), os.O_CREATE|os.O_EXCL|os.O_WRONLY, stat.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(helper, original)
	closeErr := helper.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return s.savePlan()
}

func (s *installation) launch(ctx context.Context) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.Command(s.path(executableName("installer")), HelperFlag, s.path("plan.json"))
	detach(command)
	parent, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	ready, err := command.StdoutPipe()
	if err != nil {
		parent.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		parent.Close()
		return nil, err
	}
	line, err := bufio.NewReader(ready).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		parent.Close()
		command.Wait()
		return nil, fmt.Errorf("无法启动更新程序：%s %v", strings.TrimSpace(line), err)
	}
	return parent, nil
}

// RunHelper waits until the old process releases its executable, then performs one replacement.
func RunHelper(planPath string) error {
	s, err := readInstallation(planPath)
	if err != nil {
		return err
	}
	s.plan.HelperPID = os.Getpid()
	if err := s.savePlan(); err != nil {
		return err
	}
	wait, err := waitForParent(s.plan.ParentPID)
	if err != nil {
		return err
	}
	fmt.Println("ready")
	if err := wait(); err != nil {
		return err
	}
	err = s.replaceAndStart()
	if err != nil {
		os.WriteFile(s.path("failure.txt"), []byte(err.Error()), 0600)
	}
	return err
}

func (s *installation) replaceAndStart() error {
	backup := s.path(executableName("previous"))
	if err := os.Rename(s.plan.Executable, backup); err != nil {
		return s.startPrevious(fmt.Errorf("无法替换客户端：%w", err))
	}
	if err := os.Rename(s.path(executableName("next")), s.plan.Executable); err != nil {
		if restoreErr := os.Rename(backup, s.plan.Executable); restoreErr != nil {
			return fmt.Errorf("安装失败：%v；请从 %s 恢复旧客户端：%w", err, backup, restoreErr)
		}
		return s.startPrevious(err)
	}
	if err := s.startClient(); err != nil {
		if removeErr := os.Remove(s.plan.Executable); removeErr != nil {
			return fmt.Errorf("新版本启动失败：%v；移除失败的客户端：%w", err, removeErr)
		}
		if restoreErr := os.Rename(backup, s.plan.Executable); restoreErr != nil {
			return fmt.Errorf("请从 %s 恢复旧客户端：%w", backup, restoreErr)
		}
		return s.startPrevious(err)
	}
	return nil
}

func (s *installation) startPrevious(cause error) error {
	s.plan.Failure = fmt.Sprintf("更新未完成，已恢复原版本：%v", cause)
	if err := s.savePlan(); err != nil {
		return err
	}
	return s.startClient()
}

func (s *installation) startClient() error {
	if err := os.Remove(s.path("ready")); err != nil && !os.IsNotExist(err) {
		return err
	}
	command := exec.Command(s.plan.Executable, RestartFlag, s.path("plan.json"))
	command.Dir = s.plan.WorkingDirectory
	detach(command)
	parent, err := command.StdinPipe()
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := command.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-exited:
			return fmt.Errorf("新进程在完成启动前退出：%v", err)
		case <-ticker.C:
			if _, err := os.Stat(s.path("ready")); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				command.Process.Kill()
				<-exited
				return err
			}
		}
	}
}

type Restart struct {
	Arguments []string
	Failure   string
	stage     *installation
}

func ReadRestart(planPath string) (*Restart, error) {
	s, err := readInstallation(planPath)
	if err != nil {
		return nil, err
	}
	return &Restart{Arguments: s.plan.Arguments, Failure: s.plan.Failure, stage: s}, nil
}

// Ready must only be called after data recovery, the HTTP listener and the desktop page have succeeded.
func (r *Restart) Ready(reportFailure func(string)) error {
	wait, err := waitForParent(r.stage.plan.HelperPID)
	if err != nil {
		return err
	}
	if err := os.WriteFile(r.stage.path("ready"), nil, 0600); err != nil {
		return err
	}
	go func() {
		if err := wait(); err != nil {
			reportFailure(fmt.Sprintf("客户端已启动，但无法等待旧更新进程退出：%v", err))
			return
		}
		// Windows keeps the helper mapped until its process has exited.
		if err := os.RemoveAll(r.stage.directory); err != nil {
			reportFailure(fmt.Sprintf("客户端已启动，但旧版本文件未能清理：%v", err))
		}
	}()
	return nil
}
