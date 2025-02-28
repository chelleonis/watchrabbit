Domain - core business domain logic + models for analysis system

Models - data structures
- e.g. BiomarkerFile - represents a file containing biomarker data
- AnalysisResult - outcome of analyzing a biomarker file
Events - Messages that represent something (significant) happened in the system
- e.g. FileDetectedEvent - triggers when a new file is discovered
Value objects - immutable - encapsulates domain logic
Entities - Objects w/ unique identities/lifecycles