terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
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

  explicit_auth_flows = [
    "ALLOW_USER_PASSWORD_AUTH",
    "ALLOW_REFRESH_TOKEN_AUTH"
  ]
}

resource "aws_cognito_user" "cognito_user_gofreescout" {
  user_pool_id = aws_cognito_user_pool.bco_fund_pool.id
  username     = "gofreescout"

  attributes = {
    email          = "mpbyrne@gmail.com"
    email_verified = "true"
    member_id      = "123456"
  }
}