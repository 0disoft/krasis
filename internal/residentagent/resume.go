package residentagent

import "github.com/0disoft/krasis/internal/appsettings"

// Persist only the uniquely matched live trust entry. Both GUI and CLI use this
// operation; shutdown and update handoff preserve it, explicit disconnect clears it.
func (o *Operations) rememberConnectionLocked(alias string) {
	if o.localAppData == "" || o.trust == nil {
		return
	}
	doc, err := o.trust.List()
	if err != nil {
		o.recordLocked("resume_save_failed")
		return
	}
	var resume appsettings.Resume
	for _, peer := range doc.Peers {
		if peer.Alias != alias || peer.RevokedAt != nil {
			continue
		}
		if resume.Alias != "" {
			o.recordLocked("resume_save_failed")
			return
		}
		resume = appsettings.Resume{Alias: alias, Fingerprint: peer.Fingerprint}
	}
	if resume.Alias == "" {
		return
	}
	if err = appsettings.SaveResume(o.localAppData, resume); err != nil {
		o.recordLocked("resume_save_failed")
	}
}
