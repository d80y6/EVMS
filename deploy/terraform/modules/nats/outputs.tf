output "security_group_id" {
  description = "NATS security group ID"
  value       = aws_security_group.nats.id
}

output "client_port" {
  description = "NATS client port"
  value       = 4222
}

output "cluster_port" {
  description = "NATS cluster route port"
  value       = 6222
}

output "monitoring_port" {
  description = "NATS monitoring port"
  value       = 8222
}
