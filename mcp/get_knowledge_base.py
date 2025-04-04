import os
import shutil
import subprocess
import re
import json
from git import Repo

def clean_tmp(dir_path):
    """ Clears out all contents of the specified directory except for prebuild.sh """
    for item in os.listdir(dir_path):
        item_path = os.path.join(dir_path, item)
        if item != "prebuild.sh":  # Keep prebuild.sh
            if os.path.isdir(item_path):
                shutil.rmtree(item_path)
            else:
                os.remove(item_path)

def clone_repository(repo_url, local_dir):
    """ Clone or pull the repository based on its existence. """
    if not os.path.exists(local_dir):
        print(f"Cloning repository into {local_dir}")
        Repo.clone_from(repo_url, local_dir, depth=1)
    else:
        print(f"Repository already exists at {local_dir}. Pulling latest changes...")
        repo = Repo(local_dir)
        repo.git.pull()

def setup_repositories():
    tmp_dir = ".tmp"
    os.makedirs(tmp_dir, exist_ok=True)
    clean_tmp(tmp_dir)  # Clean the temporary directory before setting up

    # Define repositories and their URLs
    repos = {
        "defang-docs": "https://github.com/DefangLabs/defang-docs.git",
        "defang": "https://github.com/DefangLabs/defang.git"
    }

    # Change to the temporary directory
    original_dir = os.getcwd()
    os.chdir(tmp_dir)

    # Clone each repository
    for repo_name, repo_url in repos.items():
        clone_repository(repo_url, repo_name)

    # Return to the original directory
    os.chdir(original_dir)

def run_prebuild_script():
    """ Run the 'prebuild.sh' script located in the .tmp directory. """
    os.chdir(".tmp")
    script_path = os.path.join("./", "prebuild.sh")  # Ensure the path is correct
    if os.path.exists(script_path):
        print("Running prebuild.sh...")
        try:
            subprocess.run(["bash", script_path], check=True)
        except subprocess.CalledProcessError as e:
            print(f"Error running prebuild.sh: {e}")
    else:
        print("prebuild.sh not found.")

def cleanup():
    """ Clean up unneeded files, preserving only 'docs' and 'blog' directories """
    os.chdir("./defang-docs")
    for item in os.listdir('.'):
        if item not in ['docs', 'blog']:  # Check if the item is not one of the directories to keep
            item_path = os.path.join('.', item)  # Construct the full path
            if os.path.isdir(item_path):
                shutil.rmtree(item_path)  # Remove the directory and all its contents
            else:
                os.remove(item_path)  # Remove the file
    print("Cleanup completed successfully.")

def parse_markdown():
    """ Parse markdown files in the current directory into JSON """
    reset_knowledge_base()  # Reset the JSON database file
    recursive_parse_directory('./.tmp/defang-docs')  # Parse markdown files in the current directory
    print("Markdown parsing completed successfully.")

def reset_knowledge_base():
    """ Resets or initializes the knowledge base JSON file. """
    with open('./knowledge_base.json', 'w') as output_file:
        json.dump([], output_file)

def parse_markdown_file_to_json(file_path):
    """ Parses individual markdown file and adds its content to JSON """
    try:
        # Load existing content if the file exists
        with open('./knowledge_base.json', 'r') as existing_file:
            json_output = json.load(existing_file)
            current_id = len(json_output) + 1  # Start ID from the next available number
    except (FileNotFoundError, json.JSONDecodeError):
        # If the file doesn't exist or is empty, start fresh
        json_output = []
        current_id = 1

    with open(file_path, 'r', encoding='utf-8') as file:
        lines = file.readlines()

    # Skip the first 5 lines
    markdown_content = "".join(lines[5:])

    # First pass: Determine headers for 'about' section
    sections = []
    current_section = {"about": [], "text": []}
    has_main_header = False

    for line in markdown_content.split('\n'):
        header_match = re.match(r'^(#{1,6}|\*\*+)\s+(.*)', line)  # Match `#`, `##`, ..., `######` and `**`
        if header_match:
            header_level = len(header_match.group(1).strip())
            header_text = header_match.group(2).strip()

            if header_level == 1 or header_match.group(1).startswith('**'):  # Treat `**` as a main header
                if current_section["about"] or current_section["text"]:
                    sections.append(current_section)
                current_section = {"about": [header_text], "text": []}
                has_main_header = True
            else:
                if has_main_header:
                    current_section["about"].append(header_text)
                else:
                    if header_level == 2:
                        if current_section["about"] or current_section["text"]:
                            sections.append(current_section)
                        current_section = {"about": [header_text], "text": []}
                    else:
                        current_section["about"].append(header_text)
        else:
            current_section["text"].append(line.strip())

    if current_section["about"] or current_section["text"]:
        sections.append(current_section)

    # Second pass: Combine text while ignoring headers and discard entries with empty 'about' or 'text'
    for section in sections:
        about = ", ".join(section["about"])
        text = " ".join(line for line in section["text"] if line)

        if about and text:  # Only insert if both 'about' and 'text' are not empty
            json_output.append({
                "id": current_id,
                "about": about,
                "text": text
            })
            current_id += 1

    # Write the augmented JSON output to knowledge_base.json
    with open('./knowledge_base.json', 'w', encoding='utf-8') as output_file:
        json.dump(json_output, output_file, indent=2, ensure_ascii=False)

def parse_cli_markdown(file_path):
    """ Parses CLI-specific markdown files """
    try:
        # Load existing content if the file exists
        with open('./knowledge_base.json', 'r') as existing_file:
            json_output = json.load(existing_file)
            current_id = len(json_output) + 1  # Start ID from the next available number
    except (FileNotFoundError, json.JSONDecodeError):
        # If the file doesn't exist or is empty, start fresh
        json_output = []
        current_id = 1

    with open(file_path, 'r', encoding='utf-8') as file:
        lines = file.readlines()

    if len(lines) < 5:
        print(f"File {file_path} does not have enough lines to parse.")
        return

    # Extract 'about' from the 5th line (index 4)
    about = lines[4].strip()

    # Combine all remaining lines after the first 5 lines into 'text'
    text_lines = lines[5:]
    text = "".join(text_lines).strip()

    # Only append if both 'about' and 'text' are not empty
    if about and text:
        json_output.append({
            "id": current_id,
            "about": about,
            "text": text
        })
        current_id += 1

    # Write the augmented JSON output to knowledge_base.json
    with open('./knowledge_base.json', 'w', encoding='utf-8') as output_file:
        json.dump(json_output, output_file, indent=2, ensure_ascii=False)

def recursive_parse_directory(root_dir):
    """ Recursively parses all markdown files in the directory. """
    for dirpath, dirnames, filenames in os.walk(root_dir):
        for filename in filenames:
            if filename.lower().endswith('.md'):
                file_path = os.path.join(dirpath, filename)
                if 'cli' in dirpath.lower() or 'cli' in filename.lower():
                    parse_cli_markdown(file_path)
                else:
                    parse_markdown_file_to_json(file_path)

if __name__ == "__main__":
    setup_repositories()
    run_prebuild_script()
    cleanup()
    os.chdir('../../')
    print(os.listdir('.'))
    parse_markdown()  # Start parsing logic after all setups
    print(os.listdir('.'))
    clean_tmp('./.tmp')
    print("All processes completed successfully.")
