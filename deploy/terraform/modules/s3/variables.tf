variable "recordings_bucket" {
  description = "Name of the S3 bucket for recordings"
  type        = string
}

variable "backups_bucket" {
  description = "Name of the S3 bucket for backups"
  type        = string
}

variable "environment" {
  description = "Environment name"
  type        = string
}
