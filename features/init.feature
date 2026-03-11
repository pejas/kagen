Feature: Repository Initialization
  As a developer
  I want to initialize my repository with kagen
  So that I can start working in an isolated environment

  Scenario: Successful initialization
    Given a directory that is a git repository
    When I run "kagen init"
    Then the file "devfile.yaml" should exist
    And the output should contain "Wrote ... devfile.yaml"
