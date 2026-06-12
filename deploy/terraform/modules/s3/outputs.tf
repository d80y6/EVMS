output "recordings_bucket_id" {
  description = "Recordings S3 bucket ID"
  value       = aws_s3_bucket.recordings.id
}

output "recordings_bucket_arn" {
  description = "Recordings S3 bucket ARN"
  value       = aws_s3_bucket.recordings.arn
}

output "backups_bucket_id" {
  description = "Backups S3 bucket ID"
  value       = aws_s3_bucket.backups.id
}

output "backups_bucket_arn" {
  description = "Backups S3 bucket ARN"
  value       = aws_s3_bucket.backups.arn
}
