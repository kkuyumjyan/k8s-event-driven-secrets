output "edsm_role_arn" {
  value = aws_iam_role.edsm_service_account.arn
}

# Output SQS Queue URL
output "sqs_queue_url" {
  description = "The HTTPS URL of the SQS Queue"
  value       = aws_sqs_queue.secrets_manager_events_queue.url
}
