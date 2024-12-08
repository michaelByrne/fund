terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket  = "tf-fund"
    key     = "fund-tfstate"
    region  = "us-west-2"
    encrypt = true
  }

  required_version = ">= 1.2.0"
}

provider "aws" {
  region = "us-west-2"
}

resource "aws_cognito_user_pool" "bco_fund_pool" {
  name = "bco-fund-pool"

  admin_create_user_config {
    allow_admin_create_user_only = true
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
  supported_identity_providers = ["COGNITO"]

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

resource "aws_cognito_user_group" "bco_admin_group" {
  name         = "bco-admin-group"
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
}

resource "aws_cognito_user" "cognito_user_gofreescout" {
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
  username     = "gofreescout"

  attributes = {
    email          = "mpbyrne@gmail.com"
    email_verified = "true"
    member_id      = "123456"
  }

  lifecycle {
    ignore_changes = [
      attributes,
    ]
  }
}

resource "aws_cognito_user" "cognito_user_michael" {
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
  username     = "michael"

  attributes = {
    email          = "mpbyrne@gmail.com"
    email_verified = "true"
    member_id      = "123456"
  }

  lifecycle {
    ignore_changes = [
      attributes,
    ]
  }
}

resource "aws_cognito_user_in_group" "gofreescout_admin_group_membership" {
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
  username     = aws_cognito_user.cognito_user_gofreescout.username
  group_name   = aws_cognito_user_group.bco_admin_group.name
}

resource "aws_cognito_user_in_group" "michael_admin_group_membership" {
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
  username     = aws_cognito_user.cognito_user_michael.username
  group_name   = aws_cognito_user_group.bco_admin_group.name
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
    effect = "Allow"
    resources = ["*"]
  }
}