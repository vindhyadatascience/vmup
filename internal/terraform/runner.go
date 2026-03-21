package terraform

import (
	"context"
	"io"

	"github.com/hashicorp/terraform-exec/tfexec"
)

type Runner struct {
	tf *tfexec.Terraform
}

func NewRunner(workDir, execPath string) (*Runner, error) {
	tf, err := tfexec.NewTerraform(workDir, execPath)
	if err != nil {
		return nil, err
	}
	return &Runner{tf: tf}, nil
}

func (r *Runner) SetOutput(stdout, stderr io.Writer) {
	r.tf.SetStdout(stdout)
	r.tf.SetStderr(stderr)
}

func (r *Runner) Init(ctx context.Context) error {
	return r.tf.Init(ctx, tfexec.Upgrade(false))
}

func (r *Runner) Apply(ctx context.Context) error {
	return r.tf.Apply(ctx)
}

func (r *Runner) Destroy(ctx context.Context) error {
	return r.tf.Destroy(ctx)
}
