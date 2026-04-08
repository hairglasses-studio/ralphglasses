# Resume Inventory - 2026-04-08

Scope: workspace audit of actual job-search resume artifacts under `~/hairglasses-studio`, excluding session-resume docs and code paths unrelated to candidate resumes.

## Canonical corpus

### Mitch

- Canonical public-facing 2026 PDF:
  - `/home/hg/hairglasses-studio/jobb/resumes/Mitch Mitchell Resume 2026.pdf`
  - Same file hash as:
    - `/home/hg/hairglasses-studio/jobb/resume.pdf`
    - `/home/hg/hairglasses-studio/docs/resume.pdf`
    - `/home/hg/hairglasses-studio/ralphglasses/resume.pdf`
    - `/home/hg/hairglasses-studio/resume.pdf`
- Older reproducible 2026 draft set:
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/resume-draft-2026.md`
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/resume-linkedin-2026.md`
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/resume-bullets.md`
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/top-achievements.md`
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/achievement-portfolio.md`
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/evidence-appendix.md`
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/Mitch_Burk_Resume_2026.pdf`
- Legacy 2025 PDF:
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/Mitch Burk - Resume 2025.pdf`

### Other tracked resumes

- `/home/hg/hairglasses-studio/jobb/resumes/Austin Potter Resume.pdf`
- `/home/hg/hairglasses-studio/jobb/resumes/Austin Potter Resume.docx`
- `/home/hg/hairglasses-studio/jobb/resumes/Michelle Batan Resume 2026.pdf`
- `/home/hg/hairglasses-studio/jobb/resumes/Oliver Resume.pdf`
- `/home/hg/hairglasses-studio/jobb/resumes/Resume Jon Downer.pdf`

## Duplicate map

### Mitch 2026 "Mitchell" PDF

- Hash: `a288fa06c02f2f927f96bf3265b9125039345f275da3ee2dbf23ce6563713613`
- Locations:
  - `/home/hg/hairglasses-studio/jobb/resumes/Mitch Mitchell Resume 2026.pdf`
  - `/home/hg/hairglasses-studio/jobb/resume.pdf`
  - `/home/hg/hairglasses-studio/docs/resume.pdf`
  - `/home/hg/hairglasses-studio/ralphglasses/resume.pdf`
  - `/home/hg/hairglasses-studio/resume.pdf`

### Mitch 2026 "Burk" PDF

- Hash: `c8af806e285f014d88b49b3db54ef4498ceab47e4a493dd8715bcf5bab29b04d`
- Locations:
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/Mitch_Burk_Resume_2026.pdf`
  - `/home/hg/hairglasses-studio/jobb/python/crabrave/resume/Mitch_Burk_Resume_2026.pdf`
  - `/home/hg/hairglasses-studio/jobb/python/crabravee/resume/Mitch_Burk_Resume_2026.pdf`
  - `/home/hg/hairglasses-studio/jobb/python/crabrave/data/gdrive/resume/Mitch_Burk_Resume_2026.pdf`
  - `/home/hg/hairglasses-studio/jobb/python/crabravee/data/gdrive/resume/Mitch_Burk_Resume_2026.pdf`

### Mitch 2025 PDF

- Hash: `55f9ed9020b8aa9327948f5f89cb05abb6fa525942466de5f39b812a8b6af545`
- Location:
  - `/home/hg/hairglasses-studio/jobb/resumes/drafts-2026/Mitch Burk - Resume 2025.pdf`

## Content snapshot

### Mitch canonical 2026 PDF

- 6 pages
- Uses preferred/public identity:
  - Name: "Mitch Not Mitchell"
  - Email: `mitchmitchellworkinquiries@gmail.com`
  - LinkedIn: `linkedin.com/in/mitchnotmitchell`
- Expanded story with:
  - summary + key achievements section
  - fuller Galileo section
  - Concourse Labs and community sections
  - stronger enterprise-customer and knowledge-graph positioning

### Mitch draft 2026 markdown/PDF set

- 2-page PDF build target generated from markdown
- Uses older identity:
  - Name: "Mitch Burk"
  - Email: `mixellburk@gmail.com`
  - LinkedIn: `linkedin.com/in/mitchburk`
- Optimized for Anthropic-era targeting and backed by:
  - `resume-bullets.md`
  - `top-achievements.md`
  - `achievement-portfolio.md`
  - `evidence-appendix.md`

### Other candidate resumes

- Austin:
  - PDF version is a broader infra/platform resume.
  - DOCX version is a more polished senior platform/SRE variant with stronger quantified bullets.
- Michelle:
  - 1-page marketing/social/content resume.
- Oliver:
  - 3-page senior DevSecOps / cloud security resume.
- Jon:
  - 3-page operations / executive support / logistics resume.

## Current jobb tracking status

### Config

- `~/.config/jobb/config.json` points `apply_profile.resume_file` to:
  - `/home/hg/hairglasses-studio/jobb/resumes/Mitch Mitchell Resume 2026.pdf`
- But the same config also contains clearly bad inferred data:
  - `apply_profile.location = "ArgoCD, CI"`
  - tenant target roles = security roles
  - tenant preferred location = `"ArgoCD, CI"`

### Mitch user overlay

- `~/.config/jobb/users/mitch/config.json` is incomplete:
  - no `resume_file`
  - no phone
  - no location
  - no website/github

### Resume DB state

- Shared DB (`~/.config/jobb/jobb.db`) contains:
  - `Mitch Mitchell Resume 2026` as base resume
  - content length observed as zero during query
- User DB (`~/.config/jobb/users/mitch/jobb.db`) contains only:
  - `test-resume-01 | Base Resume`

### Import pipeline coverage

- `jobb/cmd/import-resumes/main.go` onboards:
  - Austin
  - Michelle
  - Jon
  - Oliver
- Mitch is not included in that import command.

## Remaining work

1. Choose one canonical Mitch identity set for resumes, LinkedIn, cover letters, and config:
   - `Mitch Not Mitchell` vs `Mitch Mitchell` vs `Mitch Burk`
   - work inquiries Gmail vs personal Gmail
   - `mitchnotmitchell` vs `mitchburk` LinkedIn URL

2. Recreate the latest 6-page Mitch resume from source:
   - no markdown or build script for `Mitch Mitchell Resume 2026.pdf` was found
   - current build pipeline only reproduces `Mitch_Burk_Resume_2026.pdf`

3. Fix jobb tracking for Mitch:
   - import parsed text for the canonical Mitch resume into the mitch user DB
   - remove the placeholder `test-resume-01`
   - ensure the base resume record has parsed content

4. Repair bad hydrated config fields:
   - `apply_profile.location`
   - tenant target roles
   - tenant preferred locations

5. Collapse duplicate file copies:
   - keep one canonical Mitch PDF path
   - keep draft mirrors only where intentionally required
   - remove redundant root-level copies once callers are updated

6. Decide whether `crabrave` / `crabravee` remain active resume workspaces or become read-only mirrors:
   - both currently duplicate the same resume stack and PDF builder

7. Normalize multi-user resume source-of-truth in `jobb/resumes/`:
   - Austin has both PDF and DOCX variants
   - Michelle has a base markdown file under `jobb/data/resumes/`
   - Jon and Oliver appear file-only from this audit

8. Add a Mitch-specific import/onboarding path to `jobb/cmd/import-resumes/main.go` or replace it with a generic importer that reads from canonical resume assets instead of `/tmp/jobb-resumes/*.txt`.
