terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket  = "fund-tf-state"
    key     = "fund-tfstate"
    region  = "us-west-2"
    encrypt = true
  }

  required_version = ">= 1.2.0"
}

provider "aws" {
  region = "us-west-2"
}

variable "fund_pass_url" {
  type = string
}

variable "domain" {
  type = string
}

variable "mail_bucket" {
  type    = string
  default = "fund-mail-bucket"
}

variable "fund_images_bucket" {
  type        = string
  default     = "fund-images-bucket"
  description = "Pictures shown on fund pages. Read as FUND_IMAGES_S3_BUCKET by the app."
}

variable "donations_reports_bucket" {
  type    = string
  default = "fund-reports-bucket"
}

resource "aws_cognito_user_pool" "bco_fund_pool" {
  name = "bco-fund-pool"

  admin_create_user_config {
    allow_admin_create_user_only = true

    invite_message_template {
      email_message = "Hello {username}!\nYou're invited to the BCO Mutual Aid app. Your temporary password is {####}.\nFirst thing, you'll need to set a permanent password. Please visit ${var.fund_pass_url} to do that."
      email_subject = "help test bcofund.org"
      sms_message   = "Hello {username}! Your temporary password is {####}. You'll be prompted to change your password at login."
    }
  }

  # Cognito sends from its own default address. Pointing source_arn at
  # aws_ses_email_identity.welcome_email requires that identity to be verified
  # first -- which needs the domain's DNS records published and mail deliverable to
  # welcome@ -- and the pool cannot be created until it is.
  #
  # It would not buy throughput either: under COGNITO_DEFAULT, source_arn only
  # changes the From address and the 50 emails/day cap still applies. Raising that
  # means email_sending_account = "DEVELOPER", which needs the verified identity
  # plus an IAM role for Cognito. Worth revisiting once SES is out of the sandbox.
  email_configuration {
    email_sending_account = "COGNITO_DEFAULT"
  }

  schema {
    name                     = "member_id"
    attribute_data_type      = "String"
    mutable                  = true
    required                 = false
    developer_only_attribute = false
    string_attribute_constraints {}
  }

  account_recovery_setting {
    recovery_mechanism {
      name     = "verified_email"
      priority = 1
    }
  }
}

resource "aws_cognito_user_pool_client" "bco_pool_client" {
  name                                 = "dev-bco-pool-client"
  user_pool_id                         = aws_cognito_user_pool.bco_fund_pool.id
  generate_secret                      = false
  allowed_oauth_flows_user_pool_client = false
  supported_identity_providers         = ["COGNITO"]

  refresh_token_validity = 1
  access_token_validity  = 60
  id_token_validity      = 60

  token_validity_units {
    refresh_token = "days"
    access_token  = "minutes"
    id_token      = "minutes"
  }

  explicit_auth_flows = [
    "ALLOW_USER_PASSWORD_AUTH",
    "ALLOW_REFRESH_TOKEN_AUTH"
  ]
}

# Membership of this group is the entire admin authorisation decision -- see the
# comment on jwtauth.AdminGroup. The group is declared here; who is in it is not.
#
# Individual users and their group memberships used to be declared here too. They
# fought with the application, which creates users itself at registration: a user
# Terraform believes it owns is one Terraform will delete, and the attributes it
# writes are not the ones registration writes. Admins are now granted from the
# member page in the admin UI, which is the only place that knows who the members
# are.
#
# Bootstrapping the first admin is a one-off, and belongs in the console or the
# CLI rather than in state that gets reapplied:
#
#   aws cognito-idp admin-add-user-to-group \
#     --user-pool-id <pool> --username <user> --group-name bco-admin-group
resource "aws_cognito_user_group" "bco_admin_group" {
  name         = "bco-admin-group"
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
}

module "oidc_github" {
  source              = "unfunco/oidc-github/aws"
  version             = "1.7.1"
  attach_admin_policy = true

  github_repositories = [
    "michaelByrne/fund"
  ]

  iam_role_inline_policies = {
    "actions" : data.aws_iam_policy_document.actions.json
  }
}

data "aws_iam_policy_document" "actions" {
  statement {
    actions = [
      "s3:GetObject",
      "ec2:TerminateInstances",
      "iam:PassRole",
      "ec2:RunInstances",
    ]
    effect    = "Allow"
    resources = ["*"]
  }
}

// mail stuff

resource "aws_s3_bucket" "mail_bucket" {
  bucket = var.mail_bucket
}

resource "aws_ses_domain_identity" "fund_domain" {
  domain = var.domain
}

resource "aws_ses_domain_dkim" "fund_domain_dkim" {
  domain = aws_ses_domain_identity.fund_domain.domain
}

# Without this, SES uses amazonses.com as the envelope sender. Mail still passes
# SPF -- against Amazon's domain, not ours -- so it does not align, and DMARC
# then rests entirely on DKIM. A custom MAIL FROM aligns both.
#
# The DNS this needs is already published: an MX for mail.bcofund.org pointing at
# feedback-smtp.us-west-2.amazonses.com, and a TXT with
# "v=spf1 include:amazonses.com ~all". The subdomain is deliberate -- SPF is
# checked against the envelope sender, so the apex record covering Cloudflare's
# mail routing is unaffected and does not need amazonses added to it.
#
# UseDefaultValue on MX failure, not RejectMessage: if the records go missing,
# SES falls back to amazonses.com and mail still arrives unaligned. The
# alternative is invitations bouncing, and stale DNS is exactly the failure this
# domain has already had.
resource "aws_ses_domain_mail_from" "fund_domain_mail_from" {
  domain                 = aws_ses_domain_identity.fund_domain.domain
  mail_from_domain       = "mail.${var.domain}"
  behavior_on_mx_failure = "UseDefaultValue"
}

resource "aws_ses_email_identity" "welcome_email" {
  email = "welcome@${var.domain}"
}

resource "null_resource" "delay" {
  provisioner "local-exec" {
    command = "sleep 10"
  }
  triggers = {
    "after" = aws_s3_bucket.mail_bucket.id
  }
}

resource "aws_s3_bucket_policy" "mail_bucket_policy" {
  bucket = aws_s3_bucket.mail_bucket.id

  policy = <<POLICY
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowSESPuts",
            "Effect": "Allow",
            "Principal": {
                "Service": "ses.amazonaws.com"
            },
            "Action": "s3:PutObject",
            "Resource": "arn:aws:s3:::${aws_s3_bucket.mail_bucket.id}/*"
        }
    ]
}
POLICY
  depends_on = [
    null_resource.delay
  ]
}

resource "aws_ses_receipt_rule_set" "mail_rule_set" {
  rule_set_name = "mail-rule-set"
}

resource "aws_ses_receipt_rule" "store" {
  name          = "store"
  rule_set_name = aws_ses_receipt_rule_set.mail_rule_set.rule_set_name
  enabled       = true
  scan_enabled  = true

  s3_action {
    bucket_name       = aws_s3_bucket.mail_bucket.id
    object_key_prefix = "incoming"
    position          = 2
  }

  depends_on = [
    aws_s3_bucket_policy.mail_bucket_policy,
    aws_ses_receipt_rule.store
  ]
}

resource "aws_s3_bucket" "donations_reports" {
  bucket = var.donations_reports_bucket
}

# One bucket for every fund's picture, keyed "fund/<uuid>/<sha256>.<ext>".
#
# One bucket rather than the per-fund-per-type shape the report code uses: a key
# costs nothing, a bucket counts against an account limit, and this needs no
# CreateBucket call on the path that creates a fund.
resource "aws_s3_bucket" "fund_images" {
  bucket = var.fund_images_bucket
}

# Private, and stated rather than assumed. The objects are read back through the
# application, which is what keeps them behind the same URL, the same cache and
# the same content-addressing as everything else it serves -- so there is no
# reason for anything here to be reachable directly, and public access being off
# is a property worth pinning rather than inheriting from an account default.
resource "aws_s3_bucket_public_access_block" "fund_images" {
  bucket = aws_s3_bucket.fund_images.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# Replacing a fund's picture writes a new key and deletes the old one, so nothing
# here is versioned on purpose. This is the safety net for the delete being wrong,
# and it expires so it does not become a permanent copy of everything ever
# uploaded.
resource "aws_s3_bucket_versioning" "fund_images" {
  bucket = aws_s3_bucket.fund_images.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "fund_images" {
  bucket = aws_s3_bucket.fund_images.id

  rule {
    id     = "expire-replaced-images"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }

  depends_on = [aws_s3_bucket_versioning.fund_images]
}

# The application reads these three values as COGNITO_USER_POOL_ID,
# COGNITO_CLIENT_ID and JWK_URL. They change whenever the pool is recreated -- a new
# AWS account, or a destroy/apply -- so they are surfaced here rather than being
# copied out of the console.
output "cognito_user_pool_id" {
  description = "Set as COGNITO_USER_POOL_ID in the application environment."
  value       = aws_cognito_user_pool.bco_fund_pool.id
}

output "cognito_client_id" {
  description = "Set as COGNITO_CLIENT_ID in the application environment."
  value       = aws_cognito_user_pool_client.bco_pool_client.id
}

# Built from the pool's own endpoint rather than a hardcoded region, so it stays
# correct if the provider region ever changes.
output "jwk_url" {
  description = "Set as JWK_URL in the application environment."
  value       = "https://${aws_cognito_user_pool.bco_fund_pool.endpoint}/.well-known/jwks.json"
}

// application IAM

# The report buckets the app creates at runtime are named "<report type>.<fund uuid>",
# so the S3 grants below are scoped by prefix rather than to fixed bucket names.
# This must stay in sync with the app's ENABLED_REPORT_TYPES: a type listed there but
# missing here fails at CreateBucket when a fund is created.
variable "report_bucket_prefixes" {
  type        = list(string)
  default     = ["payments"]
  description = "Report types the app creates per-fund buckets for. Mirrors ENABLED_REPORT_TYPES."
}

# Railway has no instance role, so the application authenticates with a static key.
# Its permissions are deliberately narrower than the CI role's: no IAM, no ability to
# reach the Terraform state bucket, and no access to buckets outside the report
# prefixes.
resource "aws_iam_user" "fund_app" {
  name = "fund-app"
}

resource "aws_iam_access_key" "fund_app" {
  user = aws_iam_user.fund_app.name
}

data "aws_iam_policy_document" "fund_app" {
  # Registration, password reset and login. Scoped to this pool: the app has no
  # reason to touch another, and AdminDeleteUser is destructive enough to bound.
  #
  # The group actions are what let an admin promote another member from the admin
  # UI. Group membership is the only thing that grants admin -- see the comment on
  # jwtauth.AdminGroup -- so these three are effectively "manage who is an admin".
  statement {
    sid = "CognitoUserAdministration"

    actions = [
      "cognito-idp:AdminCreateUser",
      "cognito-idp:AdminSetUserPassword",
      "cognito-idp:AdminDeleteUser",
      "cognito-idp:AdminGetUser",
      "cognito-idp:InitiateAuth",
      "cognito-idp:AdminAddUserToGroup",
      "cognito-idp:AdminRemoveUserFromGroup",
      "cognito-idp:AdminListGroupsForUser",
    ]

    effect    = "Allow"
    resources = [aws_cognito_user_pool.bco_fund_pool.arn]
  }

  # Creating and listing the per-fund report buckets.
  statement {
    sid = "ReportBucketAdministration"

    actions = [
      "s3:CreateBucket",
      "s3:ListBucket",
      "s3:GetBucketLocation",
    ]

    effect    = "Allow"
    resources = [for prefix in var.report_bucket_prefixes : "arn:aws:s3:::${prefix}.*"]
  }

  # Reading and writing the reconciliation CSVs inside those buckets.
  statement {
    sid = "ReportObjectAccess"

    actions = [
      "s3:PutObject",
      "s3:GetObject",
    ]

    effect    = "Allow"
    resources = [for prefix in var.report_bucket_prefixes : "arn:aws:s3:::${prefix}.*/*"]
  }

  # Fund pictures: objects only, in one named bucket.
  #
  # No CreateBucket and no ListBucket. The application never enumerates this --
  # the database holds every key it will ever ask for -- so being unable to list
  # it costs nothing and means a leaked key cannot be used to find out what is
  # there.
  statement {
    sid = "FundImageObjectAccess"

    actions = [
      "s3:PutObject",
      "s3:GetObject",
      "s3:DeleteObject",
    ]

    effect    = "Allow"
    resources = ["${aws_s3_bucket.fund_images.arn}/*"]
  }
}

resource "aws_iam_user_policy" "fund_app" {
  name   = "fund-app-runtime"
  user   = aws_iam_user.fund_app.name
  policy = data.aws_iam_policy_document.fund_app.json
}

output "fund_app_access_key_id" {
  description = "Set as AWS_ACCESS_KEY_ID in the application environment."
  value       = aws_iam_access_key.fund_app.id
}

# Read with: terraform output -raw fund_app_secret_access_key
output "fund_app_secret_access_key" {
  description = "Set as AWS_SECRET_ACCESS_KEY in the application environment."
  value       = aws_iam_access_key.fund_app.secret
  sensitive   = true
}
