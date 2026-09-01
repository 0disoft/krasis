package pairing

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

const (
	OfferSchemaVersion    = "krasis.pairing-offer/v1"
	ResponseSchemaVersion = "krasis.pairing-response/v1"
	ReceiptSchemaVersion  = "krasis.pairing-receipt/v1"
	DefaultLifetime       = 5 * time.Minute
)

var (
	ErrExpired            = errors.New("pairing ceremony expired")
	ErrConfirmation       = errors.New("pairing confirmation code does not match")
	ErrLocalConfirmation  = errors.New("local pairing confirmation is required")
	ErrRemoteConfirmation = errors.New("remote pairing confirmation is invalid")
)

type Participant struct {
	IdentityFingerprint string                             `json:"identityFingerprint"`
	IdentityPublicKey   []byte                             `json:"identityPublicKey"`
	TransportPeer       transport.AuthenticatedPeerContext `json:"transportPeer"`
}

type Offer struct {
	SchemaVersion string      `json:"schemaVersion"`
	PairingID     string      `json:"pairingId"`
	CreatedAt     time.Time   `json:"createdAt"`
	ExpiresAt     time.Time   `json:"expiresAt"`
	Initiator     Participant `json:"initiator"`
}

type Response struct {
	SchemaVersion string      `json:"schemaVersion"`
	PairingID     string      `json:"pairingId"`
	Responder     Participant `json:"responder"`
}

type Receipt struct {
	SchemaVersion     string `json:"schemaVersion"`
	PairingID         string `json:"pairingId"`
	TranscriptSHA256  string `json:"transcriptSha256"`
	SignerFingerprint string `json:"signerFingerprint"`
	Signature         []byte `json:"signature"`
}

type Session struct {
	pairingID      string
	expiresAt      time.Time
	transcript     [sha256.Size]byte
	confirmation   string
	local          *productidentity.Identity
	remote         Participant
	localConfirmed bool
}

func CreateOffer(identity *productidentity.Identity, transportPeer transport.AuthenticatedPeerContext, now time.Time) (Offer, error) {
	return createOffer(identity, transportPeer, now, rand.Reader)
}

func createOffer(identity *productidentity.Identity, transportPeer transport.AuthenticatedPeerContext, now time.Time, random io.Reader) (Offer, error) {
	if now.IsZero() {
		return Offer{}, errors.New("pairing creation time is required")
	}
	participant, err := participant(identity, transportPeer)
	if err != nil {
		return Offer{}, err
	}
	var identifier [16]byte
	if _, err := io.ReadFull(random, identifier[:]); err != nil {
		return Offer{}, fmt.Errorf("generate pairing identifier: %w", err)
	}
	return Offer{
		SchemaVersion: OfferSchemaVersion,
		PairingID:     hex.EncodeToString(identifier[:]),
		CreatedAt:     now.UTC(),
		ExpiresAt:     now.UTC().Add(DefaultLifetime),
		Initiator:     participant,
	}, nil
}

func Accept(offer Offer, identity *productidentity.Identity, transportPeer transport.AuthenticatedPeerContext, now time.Time) (*Session, Response, error) {
	if err := offer.validate(now); err != nil {
		return nil, Response{}, err
	}
	responder, err := participant(identity, transportPeer)
	if err != nil {
		return nil, Response{}, err
	}
	if responder.IdentityFingerprint == offer.Initiator.IdentityFingerprint {
		return nil, Response{}, errors.New("pairing participants must have different identities")
	}
	response := Response{
		SchemaVersion: ResponseSchemaVersion,
		PairingID:     offer.PairingID,
		Responder:     responder,
	}
	session, err := newSession(offer, response, identity, offer.Initiator, now)
	return session, response, err
}

func Complete(offer Offer, response Response, identity *productidentity.Identity, now time.Time) (*Session, error) {
	if err := offer.validate(now); err != nil {
		return nil, err
	}
	if err := response.validate(); err != nil {
		return nil, err
	}
	if response.PairingID != offer.PairingID {
		return nil, errors.New("pairing response identifier does not match offer")
	}
	if identity == nil || identity.Public().Fingerprint != offer.Initiator.IdentityFingerprint ||
		!bytes.Equal(identity.PublicKey(), offer.Initiator.IdentityPublicKey) {
		return nil, errors.New("pairing offer does not belong to the local identity")
	}
	return newSession(offer, response, identity, response.Responder, now)
}

func (session *Session) ConfirmationCode() string {
	if session == nil {
		return ""
	}
	return session.confirmation
}

func (session *Session) Confirm(code string, now time.Time) (Receipt, error) {
	if session == nil || session.local == nil {
		return Receipt{}, errors.New("pairing session is not initialized")
	}
	if now.IsZero() || now.After(session.expiresAt) {
		return Receipt{}, ErrExpired
	}
	if code != session.confirmation {
		return Receipt{}, ErrConfirmation
	}
	signature, err := session.local.Sign(receiptMessage(session.pairingID, session.transcript))
	if err != nil {
		return Receipt{}, err
	}
	session.localConfirmed = true
	return Receipt{
		SchemaVersion:     ReceiptSchemaVersion,
		PairingID:         session.pairingID,
		TranscriptSHA256:  hex.EncodeToString(session.transcript[:]),
		SignerFingerprint: session.local.Public().Fingerprint,
		Signature:         signature,
	}, nil
}

func (session *Session) Finalize(remote Receipt, store *truststore.Store, alias string, allowInboundControl bool, now time.Time) (truststore.PeerRecord, error) {
	if session == nil || store == nil {
		return truststore.PeerRecord{}, errors.New("pairing session and trust store are required")
	}
	if now.IsZero() || now.After(session.expiresAt) {
		return truststore.PeerRecord{}, ErrExpired
	}
	if !session.localConfirmed {
		return truststore.PeerRecord{}, ErrLocalConfirmation
	}
	if err := remote.validate(session.pairingID, session.transcript, session.remote); err != nil {
		return truststore.PeerRecord{}, err
	}
	return store.Import(session.remote.TransportPeer, alias, allowInboundControl, now.UTC())
}

func participant(identity *productidentity.Identity, transportPeer transport.AuthenticatedPeerContext) (Participant, error) {
	if identity == nil || len(identity.PublicKey()) != ed25519.PublicKeySize {
		return Participant{}, errors.New("product identity is required")
	}
	if err := transportPeer.Validate(); err != nil {
		return Participant{}, err
	}
	return Participant{
		IdentityFingerprint: identity.Public().Fingerprint,
		IdentityPublicKey:   identity.PublicKey(),
		TransportPeer:       transportPeer,
	}, nil
}

func newSession(offer Offer, response Response, local *productidentity.Identity, remote Participant, now time.Time) (*Session, error) {
	if now.IsZero() || now.After(offer.ExpiresAt) {
		return nil, ErrExpired
	}
	transcript := transcriptDigest(offer, response)
	value := binary.BigEndian.Uint32(transcript[:4]) % 1_000_000
	return &Session{
		pairingID:    offer.PairingID,
		expiresAt:    offer.ExpiresAt,
		transcript:   transcript,
		confirmation: fmt.Sprintf("%06d", value),
		local:        local,
		remote:       remote,
	}, nil
}

func transcriptDigest(offer Offer, response Response) [sha256.Size]byte {
	buffer := bytes.NewBufferString("krasis-pairing-transcript-v1\x00")
	writeString(buffer, offer.PairingID)
	writeString(buffer, offer.CreatedAt.UTC().Format(time.RFC3339Nano))
	writeString(buffer, offer.ExpiresAt.UTC().Format(time.RFC3339Nano))
	writeParticipant(buffer, offer.Initiator)
	writeParticipant(buffer, response.Responder)
	return sha256.Sum256(buffer.Bytes())
}

func writeParticipant(buffer *bytes.Buffer, value Participant) {
	writeString(buffer, value.IdentityFingerprint)
	writeBytes(buffer, value.IdentityPublicKey)
	writeString(buffer, value.TransportPeer.CertificateSHA256)
	writeString(buffer, string(value.TransportPeer.Role))
}

func writeString(buffer *bytes.Buffer, value string) { writeBytes(buffer, []byte(value)) }

func writeBytes(buffer *bytes.Buffer, value []byte) {
	_ = binary.Write(buffer, binary.BigEndian, uint32(len(value)))
	_, _ = buffer.Write(value)
}

func receiptMessage(pairingID string, transcript [sha256.Size]byte) []byte {
	buffer := bytes.NewBufferString("krasis-pairing-confirmation-v1\x00")
	writeString(buffer, pairingID)
	writeBytes(buffer, transcript[:])
	return buffer.Bytes()
}

func (offer Offer) validate(now time.Time) error {
	if offer.SchemaVersion != OfferSchemaVersion || len(offer.PairingID) != 32 {
		return errors.New("pairing offer is invalid")
	}
	if _, err := hex.DecodeString(offer.PairingID); err != nil {
		return errors.New("pairing offer identifier is invalid")
	}
	if offer.CreatedAt.IsZero() || offer.ExpiresAt.IsZero() || !offer.ExpiresAt.After(offer.CreatedAt) ||
		offer.ExpiresAt.Sub(offer.CreatedAt) != DefaultLifetime {
		return errors.New("pairing offer lifetime is invalid")
	}
	if now.Before(offer.CreatedAt) || now.After(offer.ExpiresAt) {
		return ErrExpired
	}
	return offer.Initiator.validate()
}

func (response Response) validate() error {
	if response.SchemaVersion != ResponseSchemaVersion || response.PairingID == "" {
		return errors.New("pairing response is invalid")
	}
	return response.Responder.validate()
}

func (participant Participant) validate() error {
	if len(participant.IdentityPublicKey) != ed25519.PublicKeySize {
		return errors.New("pairing identity public key is invalid")
	}
	digest := sha256.Sum256(participant.IdentityPublicKey)
	if participant.IdentityFingerprint != hex.EncodeToString(digest[:]) {
		return errors.New("pairing identity fingerprint does not match public key")
	}
	return participant.TransportPeer.Validate()
}

func (receipt Receipt) validate(pairingID string, transcript [sha256.Size]byte, signer Participant) error {
	if receipt.SchemaVersion != ReceiptSchemaVersion || receipt.PairingID != pairingID ||
		receipt.TranscriptSHA256 != hex.EncodeToString(transcript[:]) ||
		receipt.SignerFingerprint != signer.IdentityFingerprint ||
		len(receipt.Signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(signer.IdentityPublicKey), receiptMessage(pairingID, transcript), receipt.Signature) {
		return ErrRemoteConfirmation
	}
	return nil
}
