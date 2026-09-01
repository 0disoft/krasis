package productidentity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const (
	SchemaVersion = "krasis.product-identity/v1"
	identityFile  = "identity.json"
	lockFile      = ".identity.lock"
	maxStoreBytes = 64 << 10
)

var (
	ErrIdentityNotFound = errors.New("product identity does not exist")
	ErrStoreBusy        = errors.New("product identity store is busy")
)

type PublicIdentity struct {
	SchemaVersion string    `json:"schemaVersion"`
	Fingerprint   string    `json:"fingerprint"`
	Generation    uint64    `json:"generation"`
	CreatedAt     time.Time `json:"createdAt"`
}

type identityDocument struct {
	SchemaVersion string    `json:"schemaVersion"`
	PublicKey     []byte    `json:"publicKey"`
	PrivateKey    []byte    `json:"privateKey"`
	Generation    uint64    `json:"generation"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Identity struct {
	public  PublicIdentity
	private ed25519.PrivateKey
}

func (identity *Identity) Public() PublicIdentity {
	if identity == nil {
		return PublicIdentity{}
	}
	return identity.public
}

func (identity *Identity) PublicKey() []byte {
	if identity == nil || len(identity.private) != ed25519.PrivateKeySize {
		return nil
	}
	return append([]byte(nil), identity.private.Public().(ed25519.PublicKey)...)
}

func (identity *Identity) Sign(message []byte) ([]byte, error) {
	if identity == nil || len(identity.private) != ed25519.PrivateKeySize {
		return nil, errors.New("product identity is not initialized")
	}
	return ed25519.Sign(identity.private, message), nil
}

type Store struct {
	directory string
}

func New(directory string) (*Store, error) {
	if directory == "" {
		return nil, errors.New("product identity directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve product identity directory: %w", err)
	}
	return &Store{directory: absolute}, nil
}

func (store *Store) OpenOrCreate(now time.Time) (*Identity, bool, error) {
	if now.IsZero() {
		return nil, false, errors.New("identity creation time is required")
	}
	directory, release, err := store.openLocked(true)
	if err != nil {
		return nil, false, err
	}
	defer release()
	identity, err := loadFromDirectory(directory)
	if err == nil {
		return identity, false, nil
	}
	if !errors.Is(err, ErrIdentityNotFound) {
		return nil, false, err
	}
	document, err := generateDocument(1, now.UTC())
	if err != nil {
		return nil, false, err
	}
	if err := writeDocument(directory, document, false); err != nil {
		return nil, false, err
	}
	identity, err = identityFromDocument(document)
	return identity, true, err
}

func (store *Store) Load() (*Identity, error) {
	if store == nil {
		return nil, errors.New("product identity store is not initialized")
	}
	directory, release, err := store.openLocked(false)
	if err != nil {
		return nil, err
	}
	defer release()
	return loadFromDirectory(directory)
}

func loadFromDirectory(directory *securefs.Directory) (*Identity, error) {
	file, err := directory.Open(identityFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrIdentityNotFound
		}
		return nil, fmt.Errorf("open product identity: %w", err)
	}
	defer file.Close()
	if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return nil, fmt.Errorf("validate product identity security: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStoreBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read product identity: %w", err)
	}
	if len(data) > maxStoreBytes {
		return nil, errors.New("product identity exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document identityDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode product identity: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("product identity contains trailing JSON")
	}
	return identityFromDocument(document)
}

func (store *Store) Rotate(now time.Time) (*Identity, error) {
	if now.IsZero() {
		return nil, errors.New("identity rotation time is required")
	}
	directory, release, err := store.openLocked(false)
	if err != nil {
		return nil, err
	}
	defer release()
	current, err := loadFromDirectory(directory)
	if err != nil {
		return nil, err
	}
	if current.public.Generation == ^uint64(0) {
		return nil, errors.New("identity generation exhausted")
	}
	document, err := generateDocument(current.public.Generation+1, now.UTC())
	if err != nil {
		return nil, err
	}
	if err := writeDocument(directory, document, true); err != nil {
		return nil, err
	}
	return identityFromDocument(document)
}

func (store *Store) openLocked(create bool) (*securefs.Directory, func(), error) {
	if err := store.prepareDirectory(create); err != nil {
		return nil, nil, err
	}
	directory, err := securefs.OpenDirectory(store.directory)
	if err != nil {
		return nil, nil, fmt.Errorf("open product identity directory: %w", err)
	}
	lock, created, err := openLockFile(directory)
	if err != nil {
		_ = directory.Close()
		return nil, nil, err
	}
	closeHandles := func() {
		_ = lock.Close()
		_ = directory.Close()
	}
	if created {
		if err := peercredentials.ProtectSecurityFileHandle(lock); err != nil {
			closeHandles()
			return nil, nil, err
		}
	} else if err := peercredentials.ValidateSecurityFileHandle(lock); err != nil {
		closeHandles()
		return nil, nil, err
	}
	unlock, err := acquireFileLock(lock)
	if err != nil {
		closeHandles()
		return nil, nil, err
	}
	return directory, func() {
		_ = unlock()
		closeHandles()
	}, nil
}

func openLockFile(directory *securefs.Directory) (*os.File, bool, error) {
	file, err := directory.CreateExclusive(lockFile, 0o600)
	if err == nil {
		return file, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, false, fmt.Errorf("create product identity lock: %w", err)
	}
	file, err = directory.OpenReadWrite(lockFile)
	if err != nil {
		return nil, false, fmt.Errorf("open product identity lock: %w", err)
	}
	return file, false, nil
}

func (store *Store) prepareDirectory(create bool) error {
	if store == nil {
		return errors.New("product identity store is not initialized")
	}
	info, err := os.Lstat(store.directory)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return ErrIdentityNotFound
		}
		parent := filepath.Dir(store.directory)
		if _, err := peercredentials.ValidateSecurityDirectory(parent); err != nil {
			return fmt.Errorf("validate product data root security: %w", err)
		}
		if err := os.Mkdir(store.directory, 0o700); err != nil {
			return fmt.Errorf("create product identity directory: %w", err)
		}
		if _, err := peercredentials.ProtectSecurityDirectory(store.directory); err != nil {
			_ = os.Remove(store.directory)
			return err
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect product identity directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("product identity path must be a directory")
	}
	_, err = peercredentials.ValidateSecurityDirectory(store.directory)
	return err
}

func writeDocument(directory *securefs.Directory, document identityDocument, replace bool) error {
	if err := document.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, name, err := directory.CreateTemporary(".identity-", 0o600)
	if err != nil {
		return err
	}
	defer directory.Remove(name)
	if err := peercredentials.ProtectSecurityFileHandle(temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if replace {
		err = directory.Replace(name, identityFile)
	} else {
		err = directory.PromoteNoReplace(name, identityFile)
	}
	if err != nil {
		return fmt.Errorf("promote product identity: %w", err)
	}
	return nil
}

func generateDocument(generation uint64, now time.Time) (identityDocument, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return identityDocument{}, err
	}
	return identityDocument{
		SchemaVersion: SchemaVersion,
		PublicKey:     append([]byte(nil), publicKey...),
		PrivateKey:    append([]byte(nil), privateKey...),
		Generation:    generation,
		CreatedAt:     now,
	}, nil
}

func identityFromDocument(document identityDocument) (*Identity, error) {
	if err := document.validate(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(document.PublicKey)
	return &Identity{
		public: PublicIdentity{
			SchemaVersion: SchemaVersion,
			Fingerprint:   hex.EncodeToString(digest[:]),
			Generation:    document.Generation,
			CreatedAt:     document.CreatedAt,
		},
		private: append(ed25519.PrivateKey(nil), document.PrivateKey...),
	}, nil
}

func (document identityDocument) validate() error {
	if document.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported product identity schema %q", document.SchemaVersion)
	}
	if len(document.PublicKey) != ed25519.PublicKeySize || len(document.PrivateKey) != ed25519.PrivateKeySize {
		return errors.New("product identity key material is invalid")
	}
	if !bytes.Equal(ed25519.PrivateKey(document.PrivateKey).Public().(ed25519.PublicKey), document.PublicKey) {
		return errors.New("product identity public and private keys do not match")
	}
	if document.Generation == 0 || document.CreatedAt.IsZero() {
		return errors.New("product identity generation and creation time are required")
	}
	return nil
}
