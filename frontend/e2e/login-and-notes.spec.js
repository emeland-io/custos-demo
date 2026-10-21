import { test, expect } from '@playwright/test'

const adminEmail = process.env.E2E_ADMIN_EMAIL || 'admin@notebook.test'
const password = process.env.E2E_PASSWORD || 'notebook-e2e-password'

test.beforeEach(async ({ page, baseURL }) => {
  if (!baseURL) {
    throw new Error('E2E_BASE_URL must be set to run this suite')
  }
  await page.goto('/notes')
  await page.waitForSelector('input[name="login"]', { timeout: 15000 })
  await page.fill('input[name="login"]', adminEmail)
  await page.fill('input[name="password"]', password)
  await page.click('button[type="submit"]')
  await page.waitForURL('**/notes**')
})

test('creates a note and sees it rendered', async ({ page }) => {
  await page.click('button:text-is("New note")')
  await page.fill('input[placeholder="Title"]', 'Playwright Note')
  await page.fill('textarea', '# Hello from Playwright')
  await page.click('button:text-is("Create")')

  await expect(page.locator('h2')).toHaveText('Playwright Note')
})

test('edits and deletes a note', async ({ page }) => {
  // The note's own title shows up verbatim in the sidebar list too, so
  // it must not contain any button's label as a substring: Playwright's
  // unquoted `text=` selector matches case-insensitively by substring,
  // and the sidebar (which lists this title) sits before the detail
  // pane in DOM order, so e.g. a note titled "To Edit" makes
  // `page.click('text=Edit')` click the sidebar list item instead of
  // the actual Edit button. Scoping every click to `button:text-is(...)`
  // (exact match, button elements only) avoids the whole class of bug.
  await page.click('button:text-is("New note")')
  await page.fill('input[placeholder="Title"]', 'Sample Note')
  await page.fill('textarea', 'original content')
  await page.click('button:text-is("Create")')

  await page.click('button:text-is("Edit")')
  // The sidebar's "Search notes..." input is always on the page
  // alongside the note detail pane, so a bare `input` selector would
  // match it instead of the edit-title field; the edit-title input is
  // the only one with no placeholder attribute.
  await page.fill('input:not([placeholder])', 'Edited Title')
  await page.click('button:text-is("Save")')
  await expect(page.locator('h2')).toHaveText('Edited Title')

  await page.click('button:text-is("Delete")')
  // After a successful delete, onDeleted navigates to the bare /notes
  // route, which renders no h2 at all (NotesView's placeholder "Select
  // a note..." text) rather than an h2 with different content —
  // `expect(locator).not.toContainText()` requires the locator to
  // resolve to an element to check, so it fails (rather than
  // vacuously passing) when the element is genuinely absent. Assert
  // the actual post-delete UI state instead.
  await expect(page.getByText('Select a note from the list.')).toBeVisible()
})

test('adds and deletes a comment', async ({ page }) => {
  await page.click('button:text-is("New note")')
  await page.fill('input[placeholder="Title"]', 'Discussion Note')
  await page.fill('textarea', 'content')
  await page.click('button:text-is("Create")')

  // Wait for the note-detail view to actually be the current page
  // before touching its comment composer: the create form's own
  // textarea is still a live, actionable match for a bare `textarea`
  // selector until the SPA navigation finishes swapping views, so
  // filling immediately after the click can silently race into it
  // and discard the input when that form unmounts.
  await expect(page.locator('h2')).toHaveText('Discussion Note')

  // The comment composer's MarkdownEditor textarea carries no
  // placeholder attribute; it's the only textarea on the page once
  // the note exists and isn't being edited.
  await page.fill('textarea', 'Nice note!')
  await page.click('button:text-is("Comment")')
  await expect(page.locator('text=Nice note!')).toBeVisible()

  // The note's own Delete button and the comment's Delete button are
  // both present; select the last match to get the comment's.
  await page.locator('button:text-is("Delete")').last().click()
  await expect(page.locator('text=Nice note!')).not.toBeVisible()
})
