package damv1

type AlertRule struct {
	ID            string
	CameraID      string
	Name          string
	ObjectType    string
	Zone          string
	MinConfidence float64
	Action        string
	Enabled       bool
	CreatedAt     string
}

type ListAlertRulesRequest struct {
	CameraID string
}

type ListAlertRulesResponse struct {
	Rules []*AlertRule
}

type CreateAlertRuleRequest struct {
	CameraID      string
	Name          string
	ObjectType    string
	Zone          string
	MinConfidence float64
	Action        string
}

type UpdateAlertRuleRequest struct {
	ID            string
	Name          string
	ObjectType    string
	Zone          string
	MinConfidence float64
	Action        string
	Enabled       bool
}

type DeleteAlertRuleRequest struct {
	ID string
}

type DeleteAlertRuleResponse struct {
	Success bool
}
