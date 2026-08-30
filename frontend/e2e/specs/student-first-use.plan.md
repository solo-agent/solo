# Student First-Use Retention Test Plan

## Application Overview

Solo should let a first-time non-programmer understand the product, register, reach a useful personal conversation, and see a small recommended starting point before any local runtime setup. Returning users must keep their own Workspace and seven-day login.

## Test Scenarios

### 1. First useful screen

**Seed:** `e2e/student-first-use-seed.spec.ts`

#### 1.1. chinese-registration-opens-lucy

**File:** `e2e/student-first-use-registration.spec.ts`

**Steps:**
  1. Open Solo in a fresh browser whose preferred language is Chinese.
    - expect: The home page and registration entry are Chinese.
    - expect: A visible language selector allows switching languages.
  2. Register a new test account through the real registration form.
    - expect: Registration finishes on the new user’s personal Lucy Channel, not a blank dashboard or public Workspace.
    - expect: The “tell us what you want to do” card is visible before computer setup.
  3. Enter “做一个记录喝水的简单网页” and request a recommendation.
    - expect: Solo recommends the one-Agent starter web-page template.
    - expect: The goal is not secretly written into Channel messages.
  4. Open the template library.
    - expect: The default list contains the three starter templates and hides professional multi-Agent templates until requested.

### 2. Returning session

**Seed:** `e2e/student-first-use-seed.spec.ts`

#### 2.1. refresh-and-personal-workspace-are-stable

**File:** `e2e/student-first-use-session.spec.ts`

**Steps:**
  1. Register one real test account and retain its refresh token.
    - expect: Two concurrent refresh requests both succeed with the same seven-day refresh token.
    - expect: PostgreSQL still contains one valid session for that token.
  2. Simulate a browser that previously remembered the public Workspace, then log in as the test account.
    - expect: Solo selects that user’s personal Workspace.
    - expect: The dashboard opens Lucy instead of a blank or unrelated public Channel.
