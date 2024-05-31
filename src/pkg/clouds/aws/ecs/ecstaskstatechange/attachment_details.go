package ecstaskstatechange

type AttachmentDetails struct {
	Id                    string                      `json:"id,omitempty"`
	AttachmentDetailsType string                      `json:"type,omitempty"`
	Status                string                      `json:"status,omitempty"`
	Details               []AttachmentDetails_details `json:"details,omitempty"`
}

func (a *AttachmentDetails) SetId(id string) {
	a.Id = id
}

func (a *AttachmentDetails) SetAttachmentDetailsType(attachmentDetailsType string) {
	a.AttachmentDetailsType = attachmentDetailsType
}

func (a *AttachmentDetails) SetStatus(status string) {
	a.Status = status
}

func (a *AttachmentDetails) SetDetails(details []AttachmentDetails_details) {
	a.Details = details
}
