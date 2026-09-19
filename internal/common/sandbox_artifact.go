package common

// SandboxArtifactBucket returns the object-storage bucket that hosts
// CodeExec sandbox artifacts. The uploader (agent tool layer) and the
// authenticated serving route (/api/v1/documents/artifact/<name>) must
// resolve to the same bucket, so both read it from here.
func SandboxArtifactBucket() string {
	if bucket := GetEnv(EnvSandboxArtifactBucket); bucket != "" {
		return bucket
	}
	return "sandbox-artifacts"
}
