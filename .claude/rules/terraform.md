---
paths:
  - "**/*.tf"
---

# Terraform rules

- Format with `terraform fmt`; lint with `tflint`.
- Definition jump/completion is provided by `terraform-ls`.
- The AWS ruleset (`tflint-ruleset-aws`) is NOT bundled in this repo. Install it per Terraform project via
  a project-local `.tflint.hcl` + `tflint --init`, not globally here.
