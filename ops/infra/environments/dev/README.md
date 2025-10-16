# Dev Environment (Terraform)

Usage:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
cd ops/infra/environments/dev
terraform init
terraform plan -var project_id=<gcp-project>
```

The backend is configured to store state in `qazna-terraform-state/dev`. Adjust bucket/prefix before first run.
