package auth

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"

	"github.com/square/p2/pkg/logging"
)

// TestSigned is a stand-in for an auth.Manifest or auth.Digest.
type TestSigned struct {
	Id        string
	Plaintext []byte
	Signature []byte
}

func (s TestSigned) ID() string {
	return s.Id
}

func (s TestSigned) SignatureData() ([]byte, []byte) {
	return s.Plaintext, s.Signature
}

// The testHarness groups setup functions together to reduce error
// handling logic at their call site.
type testHarness struct {
	Err error
}

func (h *testHarness) loadEntities() []*openpgp.Entity {
	if h.Err != nil {
		return nil
	}
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(testUsers))
	if err != nil {
		h.Err = err
		return nil
	}
	return keyring
}

func (h *testHarness) signMessage(msg []byte, ents []*openpgp.Entity) [][]byte {
	if h.Err != nil {
		return nil
	}
	sigs := make([][]byte, len(ents))
	for i, ent := range ents {
		buf := bytes.NewBuffer([]byte{})
		err := openpgp.DetachSign(buf, ent, bytes.NewReader(msg), nil)
		if err != nil {
			h.Err = fmt.Errorf("signing message: %s", err)
			return nil
		}
		sigs[i] = buf.Bytes()
	}
	return sigs
}

func (h *testHarness) tempFile() string {
	if h.Err != nil {
		return ""
	}
	f, err := ioutil.TempFile("", "policy_test")
	if err != nil {
		h.Err = fmt.Errorf("creating temp keyring: %s", err)
		return ""
	}
	defer f.Close()
	return f.Name()
}

func rm(t *testing.T, filename string) {
	if filename != "" {
		err := os.Remove(filename)
		if err != nil {
			t.Log(err)
		}
	}
}

func (h *testHarness) saveKeys(ents []*openpgp.Entity, filename string) {
	if h.Err != nil {
		return
	}
	f, err := os.OpenFile(filename, os.O_WRONLY, 0)
	if err != nil {
		h.Err = fmt.Errorf("opening temp file: %s", err)
		return
	}
	defer f.Close()
	armorWriter, err := armor.Encode(f, openpgp.PublicKeyType, map[string]string{})
	if err != nil {
		h.Err = fmt.Errorf("creating ASCII Armor encoder: %s", err)
		return
	}
	defer armorWriter.Close()
	for _, ent := range ents {
		err = ent.SerializePrivate(armorWriter, nil)
		if err != nil {
			h.Err = fmt.Errorf("writing keyring: %s", err)
			return
		}
	}
}

// Test that the KeyringPolicy can load a keyring
func TestKeyring(t *testing.T) {
	// Setup three entities, two of which are saved to a keyring file
	h := testHarness{}
	msg := []byte("Hello World!")
	ents := h.loadEntities()
	sigs := h.signMessage(msg, ents)
	keyfile := h.tempFile()
	defer rm(t, keyfile)
	h.saveKeys(ents[:2], keyfile)
	if h.Err != nil {
		t.Error(h.Err)
		return
	}

	policy, err := LoadKeyringPolicy(
		keyfile,
		map[string][]string{
			"restricted": {fmt.Sprintf("%X", ents[1].PrimaryKey.Fingerprint)},
		},
	)
	if err != nil {
		t.Error("creating keyring policy:", err)
		return
	}
	logger := logging.TestLogger()

	// Key in keyring signs the message
	err = policy.AuthorizePod(TestSigned{"foo", msg, sigs[0]}, logger)
	if err != nil {
		t.Error("error authorizing pod manifest:", err)
	}

	// Key not in keyring signs the message
	err = policy.AuthorizePod(TestSigned{"foo", msg, sigs[2]}, logger)
	if err == nil {
		t.Error("accepted unauthorized signature")
	}

	// Verify preparer authorization policy
	err = policy.AuthorizePod(TestSigned{"restricted", msg, sigs[1]}, logger)
	if err != nil {
		t.Error("error authorizing pod manifest:", err)
	}
	err = policy.AuthorizePod(TestSigned{"restricted", msg, sigs[0]}, logger)
	if err == nil {
		t.Error("accepted unauthorized signature")
	}
}

// Test that FileKeyringPolicy can reload a keyring file when it
// changes.
func TestKeyAddition(t *testing.T) {
	h := testHarness{}
	msg := []byte("Testing 1, 2, 3")
	// two entities
	ents := h.loadEntities()[:2]
	// that both sign the message
	sigs := h.signMessage(msg, ents)
	// and a temporary file to hold the keyring
	keyfile := h.tempFile()
	defer rm(t, keyfile)
	h.saveKeys(ents[:1], keyfile)
	if h.Err != nil {
		t.Error(h.Err)
		return
	}

	policy, err := NewFileKeyringPolicy(keyfile, nil)
	if err != nil {
		t.Errorf("%s: error loading keyring: %s", keyfile, err)
		return
	}
	logger := logging.TestLogger()

	// Keyring contains only ents[0]
	err = policy.AuthorizePod(TestSigned{"test", msg, sigs[0]}, logger)
	if err != nil {
		t.Error("expected authorized, got error:", err)
	}
	err = policy.AuthorizePod(TestSigned{"test", msg, sigs[1]}, logger)
	if err == nil {
		t.Error("expected failure, got authorization")
	}

	// Update the keyring file with both keys. The sleep is needed to
	// make it much more likely that the mtime on the file differs.
	time.Sleep(time.Second)
	h.saveKeys(ents, keyfile)
	err = policy.AuthorizePod(TestSigned{"test", msg, sigs[0]}, logger)
	if err != nil {
		t.Error("expected authorized, got error:", err)
	}
	err = policy.AuthorizePod(TestSigned{"test", msg, sigs[1]}, logger)
	if err != nil {
		t.Error("expected authorized, got error:", err)
	}
}
