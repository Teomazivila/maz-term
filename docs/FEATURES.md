# DevOps Terminal Dashboard - Features Guide

This document provides a detailed overview of the features available in the DevOps Terminal Dashboard.

## System Monitoring

The System Monitoring tab provides real-time metrics about your local system:

- **CPU Usage**: Track overall CPU utilization and per-core usage
- **Memory Usage**: Monitor total, used, and free memory
- **Disk Usage**: Track filesystem utilization across mounted volumes
- **Network Statistics**: Monitor network interface activity

## HTTP Endpoint Monitoring

The HTTP Endpoint Monitoring tab allows you to track the status of important web services:

- **Endpoint Status**: Up/down status of configured HTTP endpoints
- **Response Time**: Track and visualize response time
- **Historical Data**: Response time trends over the monitoring session

## Git Repository Status

The Git Repository tab provides comprehensive information about your Git repositories:

### Repository Overview
- **Current Branch**: Display the currently checked out branch
- **Commit Count**: Total number of commits in the repository
- **Last Commit Time**: When the most recent commit was made
- **Modified Files**: Count of files with uncommitted changes
- **Pending Commits**: Commits that have not yet been pushed to the remote

### Commit History
- **Recent Commits**: List of the 10 most recent commits
- **Commit Details**:
  - Short commit hash
  - Author information
  - Commit date and time
  - Commit message

## Configuration Options

The dashboard can be extensively configured via the config.yaml file:

### General Settings
- **Refresh Interval**: How frequently to update the data
- **Theme**: Color theme for the interface
- **History Retention**: How long to retain historical data

### Layout Configuration
- **Tab Definition**: Create and name custom dashboard tabs
- **Panel Selection**: Choose which panels to display in each tab

### Integration Settings
- **HTTP Endpoints**: Configure endpoints to monitor
- **Git Repositories**: Configure repositories to track

## Keyboard Controls

Navigation and interaction is done entirely through keyboard controls:

- **q**: Quit the application
- **←/→** or **h/l**: Navigate between tabs
- **1-9**: Jump directly to tabs 1-9
- **Resize**: The UI automatically adapts to terminal size changes

## Extending the Dashboard

The dashboard is designed to be extensible in the following ways:

- **Configuration-based**: Add new endpoints and repositories through config files
- **Plugin Architecture**: Future support for custom plugins
- **Open API**: Interfaces for creating custom collectors and visualizations 