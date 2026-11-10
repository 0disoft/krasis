package wininput

import "errors"

// Shared classification vocabulary. Platform backends retain their own causes.
var (
	ErrSourceCaptureLostForeground             = errors.New("source wheel capture lost foreground")
	ErrSourceCaptureUnregisterFailed           = errors.New("source wheel capture unregister failed")
	ErrSourceCaptureCommandTimeout             = errors.New("source wheel capture command timed out")
	ErrSourceCaptureActivationSetup            = errors.New("source capture activation setup failed")
	ErrSourceCaptureActivationAPIRejected      = errors.New("source capture foreground activation was rejected")
	ErrSourceCaptureActivationVerificationLost = errors.New("source capture foreground activation lost verification")
	ErrSourceCaptureThreadAttachFailed         = errors.New("source capture foreground thread attach failed")
	ErrSourceCaptureThreadDetachFailed         = errors.New("source capture foreground thread detach failed")
	ErrSourceFocusRestoreAPIRejected           = errors.New("source focus restore was rejected")
	ErrSourceFocusRestoreVerificationLost      = errors.New("source focus restore lost verification")
	ErrSourceFocusRestoreThreadAttachFailed    = errors.New("source focus restore thread attach failed")
	ErrSourceFocusRestoreThreadDetachFailed    = errors.New("source focus restore thread detach failed")
	ErrSourceFocusReleaseCaptureRetained       = errors.New("source capture window retained foreground after hide")
	ErrSourceForegroundUnavailable             = errors.New("source foreground window unavailable")
	ErrSourceForegroundMissingBeforeShow       = errors.New("source foreground missing before capture show")
	ErrSourceForegroundMissingAfterShow        = errors.New("source foreground missing after capture show")
	ErrSourceForegroundMissingAfterRequest     = errors.New("source foreground missing after activation request")
	ErrSourceForegroundThreadUnavailable       = errors.New("source foreground thread unavailable")
	ErrSourceForegroundProbeFailed             = errors.New("source foreground response probe failed")
	ErrSourceCaptureRollbackFailed             = errors.New("source capture rollback unverified")
)
