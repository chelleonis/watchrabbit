package analyzer

type DescriptiveResultData struct {
	FilePath    string                 `json:filePath"`
	AnalysisID  string                 `json:"analysisId"`
	ContentType string                 `json:"ContentType"`
	Data        []byte                 `json:"data"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// TODO: analysis connection to R backend using roger/Rserve
// FOR NOW, we will use os/exec for PoC
type DescriptiveService struct {
	// Path to R executable
	RExecutable string
	// Directory containing R scripts
	ScriptsDir string
	// Timeout for R script execution in seconds
	Timeout int
}

func NewDescriptiveService(rExecutable, scriptsDir string, timeoutSeconds int) *DescriptiveService {
	return &DescriptiveService{}
}

func (s *Service) Analyze(filePath string) (*DescriptiveResultData, error) {

}