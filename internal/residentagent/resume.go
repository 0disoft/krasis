package residentagent

import "github.com/0disoft/krasis/internal/appsettings"

// Persist only the uniquely matched live trust entry. Both GUI and CLI use this
// operation; shutdown and update handoff preserve it, explicit disconnect clears it.
func (o *Operations) rememberConnectionLocked(alias, fingerprint string) bool {
	if o.localAppData == "" {
		return true
	}
	fail := func() bool {
		o.resumeState = "FAILED"
		o.recordLocked("resume_save_failed")
		return false
	}
	if o.trust == nil {
		return fail()
	}
	doc, err := o.trust.List()
	if err != nil {
		return fail()
	}
	var resume appsettings.Resume
	for _, peer := range doc.Peers {
		if peer.Alias != alias || peer.RevokedAt != nil {
			continue
		}
		if resume.Alias != "" {
			return fail()
		}
		if fingerprint != "" && peer.Fingerprint != fingerprint {
			return fail()
		}
		resume = appsettings.Resume{Alias: alias, Fingerprint: peer.Fingerprint}
	}
	if resume.Alias == "" {
		return fail()
	}
	if err = appsettings.SaveResume(o.localAppData, resume); err != nil {
		return fail()
	}
	o.resumeState = "SAVED"
	return true
}
