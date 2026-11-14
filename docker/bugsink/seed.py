# SPDX-License-Identifier: GPL-3.0-or-later
# Development-only seed: every key and token below is public.
import uuid

from bsmain.models import AuthToken
from projects.models import Project
from teams.models import Team

PROJECTS = [("sample-python", "1" * 32), ("sample-js", "2" * 32)]
ALL_CAPS = [
    "issues_read", "events_read", "issues_comment", "issues_triage", "issues_delete",
    "releases_read", "releases_create", "debug_files_upload",
    "projects_read", "projects_manage", "teams_read", "teams_manage",
]
READ = ["projects_read", "issues_read", "events_read"]
TOKENS = [
    ("a" * 40, "GLOBAL_READ", None, READ),
    ("b" * 40, "GLOBAL_TRIAGE", None, READ + ["issues_triage", "issues_comment"]),
    ("c" * 40, "PROJECT_READ", "sample-python", READ),
    ("d" * 40, "NO_CAPS", None, []),
]

for _, description, _, capabilities in TOKENS:
    unknown = sorted(set(capabilities) - set(ALL_CAPS))
    assert not unknown, f"token {description} lists unknown capabilities {unknown}"

team, _ = Team.objects.get_or_create(name="Demo")
projects = {}
for expected_id, (slug, key) in enumerate(PROJECTS, start=1):
    project, _ = Project.objects.get_or_create(
        slug=slug, defaults={"name": slug, "team": team, "sentry_key": uuid.UUID(key)})
    assert project.id == expected_id, f"{slug} has id {project.id}, expected {expected_id}"
    projects[slug] = project

for value, description, project_slug, capabilities in TOKENS:
    token, _ = AuthToken.objects.update_or_create(token=value, defaults={
        "description": description,
        "is_project_bound": project_slug is not None,
        "project": projects.get(project_slug),
        **{name: name in capabilities for name in ALL_CAPS},
    })
    token.full_clean()
print("seeded", sorted(projects), [t[1] for t in TOKENS])
