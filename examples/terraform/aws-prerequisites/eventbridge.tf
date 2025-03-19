# Create an EventBridge Target
resource "aws_cloudwatch_event_target" "secrets_manager_target" {
  rule      = aws_cloudwatch_event_rule.secrets_manager_rule.name
  arn       = aws_sqs_queue.secrets_manager_events_queue.arn
}

# Create an EventBridge Rule
resource "aws_cloudwatch_event_rule" "secrets_manager_rule" {
  name        = "secrets-manager-events"
  description = "Capture all Secrets Manager API calls from CloudTrail"

  event_pattern = <<EOF
{
  "source": ["aws.secretsmanager"],
  "detail-type": ["AWS API Call via CloudTrail"]
}
EOF
}