package desktop

import "testing"

func TestWorkspaceOwnershipAndRelease(t *testing.T) {
	directory := t.TempDir()
	release, running, err := AcquireWorkspace(directory)
	if err != nil || running {
		t.Fatalf("first launch: running=%v, err=%v", running, err)
	}
	t.Cleanup(release)
	if _, running, err := AcquireWorkspace(directory); err != nil || !running {
		t.Fatalf("duplicate launch: running=%v, err=%v", running, err)
	}
	otherRelease, running, err := AcquireWorkspace(t.TempDir())
	if err != nil || running {
		t.Fatalf("separate workspace: running=%v, err=%v", running, err)
	}
	otherRelease()
	release()
	releaseAgain, running, err := AcquireWorkspace(directory)
	if err != nil || running {
		t.Fatalf("restart after exit: running=%v, err=%v", running, err)
	}
	releaseAgain()
}
