#!/usr/bin/env ruby

#
# This is a deploy script for a Digital Ocean installation of the app like the
# one in `terraform/`. Currently this is an experiment only -- the actual app
# is deployed on Heroku.
#

EXEC_DIR = "/usr/local/passages-signup/"
PROJECT_ROOT = File.expand_path("../..", __FILE__)
SERVICE_NAME = "passages-signup"
TARGETS = [
  "passages-signup-0.do.brandur.org"
].freeze

#
# Main
#

def main
  message "Building binary"
  `GOOS=linux GOARCH=amd64 go build -o passages-signup #{PROJECT_ROOT}`

  TARGETS.each do |target|
    message "Uploading to #{target}"
    run "scp passages-signup root@#{target}:#{EXEC_DIR}passages-signup-new"

    # Not even close to a graceful restart.
    message "Stopping #{SERVICE_NAME}"
    run "ssh root@#{target} supervisorctl stop #{SERVICE_NAME}"

    message "Moving new binary into place"
    run "ssh root@#{target} mv #{EXEC_DIR}passages-signup-new #{EXEC_DIR}passages-signup"

    message "Starting #{SERVICE_NAME}"
    run "ssh root@#{target} supervisorctl start #{SERVICE_NAME}"
  end
end

#
# Helpers
#

def dry_run?
  ENV["DRY_RUN"] == "true"
end

def message(str)
  puts(str)
end

def run(str)
  puts(str)
  `#{str}` unless dry_run?
end

#
# Run
#

message "Executing dry run" if dry_run?
main
