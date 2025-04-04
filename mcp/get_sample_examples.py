#!/usr/bin/env python
import os
import json
import shutil
import tempfile
import subprocess
import yaml
import re

def clone_repo(repo_url, target_dir):
    """Clone the repository to a temporary directory."""
    print(f"Cloning {repo_url} to {target_dir}...")
    subprocess.run(["git", "clone", repo_url, target_dir], check=True)
    print("Repository cloned successfully.")

def get_technologies(dockerfile_content, compose_content):
    """Extract technologies used in the project based on Dockerfile and compose file."""
    technologies = []
    
    # Check for common base images in Dockerfile
    if "FROM python" in dockerfile_content:
        technologies.append("Python")
    if "FROM node" in dockerfile_content:
        technologies.append("Node.js")
    if "FROM golang" in dockerfile_content or "FROM golang:" in dockerfile_content:
        technologies.append("Go")
    if "FROM php" in dockerfile_content:
        technologies.append("PHP")
    if "FROM ruby" in dockerfile_content:
        technologies.append("Ruby")
    if "FROM rust" in dockerfile_content:
        technologies.append("Rust")
    
    # Check for frameworks and libraries in Dockerfile
    if "pip install" in dockerfile_content and "flask" in dockerfile_content.lower():
        technologies.append("Flask")
    if "pip install" in dockerfile_content and "django" in dockerfile_content.lower():
        technologies.append("Django")
    if "npm install" in dockerfile_content and "react" in dockerfile_content.lower():
        technologies.append("React")
    if "npm install" in dockerfile_content and "express" in dockerfile_content.lower():
        technologies.append("Express.js")
    if "npm install" in dockerfile_content and "next" in dockerfile_content.lower():
        technologies.append("Next.js")
    
    # Check compose file for services
    if compose_content:
        if "postgres" in compose_content.lower():
            technologies.append("PostgreSQL")
        if "mysql" in compose_content.lower():
            technologies.append("MySQL")
        if "redis" in compose_content.lower():
            technologies.append("Redis")
        if "mongodb" in compose_content.lower():
            technologies.append("MongoDB")
    
    return list(set(technologies))  # Remove duplicates

def generate_description(project_name, technologies):
    """Generate a simple description based on project name and technologies."""
    tech_str = ", ".join(technologies)
    return f"A {tech_str} application that demonstrates how to deploy a {project_name} project with Defang."

def process_sample_directory(sample_dir):
    """Process a sample directory and extract relevant information."""
    project_name = os.path.basename(sample_dir)
    print(f"Processing sample: {project_name}")
    
    # Find compose file
    compose_file = None
    for filename in ["compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"]:
        potential_file = os.path.join(sample_dir, filename)
        if os.path.exists(potential_file):
            compose_file = potential_file
            break
    
    # Find Dockerfile
    dockerfile = None
    for root, _, files in os.walk(sample_dir):
        for file in files:
            if file == "Dockerfile":
                dockerfile = os.path.join(root, file)
                break
        if dockerfile:
            break
    
    if not compose_file and not dockerfile:
        print(f"Skipping {project_name}: No compose file or Dockerfile found")
        return None
    
    result = {"projectName": project_name}
    
    # Extract compose content
    if compose_file:
        with open(compose_file, 'r') as f:
            compose_content = f.read()
            result["compose"] = compose_content
    else:
        result["compose"] = ""
    
    # Extract Dockerfile content
    if dockerfile:
        with open(dockerfile, 'r') as f:
            dockerfile_content = f.read()
            result["dockerfile"] = dockerfile_content
    else:
        result["dockerfile"] = ""
    
    # Generate technologies and description
    technologies = get_technologies(result.get("dockerfile", ""), result.get("compose", ""))
    result["technologies"] = technologies
    result["description"] = generate_description(project_name, technologies)
    
    return result

def main():
    repo_url = "https://github.com/DefangLabs/samples"
    output_file = "samples_examples.json"
    
    with tempfile.TemporaryDirectory() as temp_dir:
        # Clone the repository
        clone_repo(repo_url, temp_dir)
        
        # Process only the /samples directory
        samples_dir = os.path.join(temp_dir, "samples")
        if not os.path.exists(samples_dir):
            print(f"Error: samples directory not found in {temp_dir}")
            return
        
        # Get all subdirectories in the samples directory
        sample_dirs = [os.path.join(samples_dir, d) for d in os.listdir(samples_dir) 
                      if os.path.isdir(os.path.join(samples_dir, d))]
        
        # Process each sample directory
        results = []
        for sample_dir in sample_dirs:
            sample_data = process_sample_directory(sample_dir)
            if sample_data:
                results.append(sample_data)
        
        # Write results to JSON file
        with open(output_file, 'w') as f:
            json.dump(results, f, indent=2)
        
        print(f"Successfully processed {len(results)} samples. Results saved to {output_file}")

if __name__ == "__main__":
    main()
