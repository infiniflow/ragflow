#!/usr/bin/env node

/**
 * Validation script for Paperless-ngx UI integration
 * This script checks that all necessary UI components are properly configured
 */

const fs = require('fs');
const path = require('path');

// Colors for console output
const colors = {
  green: '\x1b[32m',
  red: '\x1b[31m',
  yellow: '\x1b[33m',
  reset: '\x1b[0m',
};

function log(message, color = 'reset') {
  console.log(`${colors[color]}${message}${colors.reset}`);
}

function checkFileExists(filePath, description) {
  const exists = fs.existsSync(filePath);
  if (exists) {
    log(`✓ ${description}`, 'green');
  } else {
    log(`✗ ${description} - File not found: ${filePath}`, 'red');
  }
  return exists;
}

function checkFileContains(filePath, searchString, description) {
  try {
    const content = fs.readFileSync(filePath, 'utf8');
    const contains = content.includes(searchString);
    if (contains) {
      log(`✓ ${description}`, 'green');
    } else {
      log(`✗ ${description} - String not found: ${searchString}`, 'red');
    }
    return contains;
  } catch (error) {
    log(`✗ ${description} - Error reading file: ${error.message}`, 'red');
    return false;
  }
}

function runValidation() {
  log('\n=== Paperless-ngx UI Integration Validation ===\n', 'yellow');

  const webDir = __dirname;  // Changed from path.join(__dirname, '..')
  let allPassed = true;

  // Check 1: SVG Icon exists
  allPassed &= checkFileExists(
    path.join(webDir, 'src/assets/svg/data-source/paperless-ngx.svg'),
    'SVG icon file exists'
  );

  // Check 2: DataSourceKey enum includes PAPERLESS_NGX
  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    "PAPERLESS_NGX = 'paperless_ngx'",
    'DataSourceKey enum includes PAPERLESS_NGX'
  );

  // Check 3: generateDataSourceInfo includes Paperless-ngx
  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    '[DataSourceKey.PAPERLESS_NGX]: {',
    'generateDataSourceInfo includes Paperless-ngx entry'
  );

  // Check 4: Form fields are defined
  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    "name: 'config.base_url'",
    'Form field for base_url is defined'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    "name: 'config.credentials.api_token'",
    'Form field for api_token is defined'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    "name: 'config.verify_ssl'",
    'Form field for verify_ssl is defined'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    "name: 'config.batch_size'",
    'Form field for batch_size is defined'
  );

  // Check 5: Default values are set
  allPassed &= checkFileContains(
    path.join(webDir, 'src/pages/user-setting/data-source/constant/index.tsx'),
    'source: DataSourceKey.PAPERLESS_NGX',
    'Default values include source field'
  );

  // Check 6: English translations exist
  allPassed &= checkFileContains(
    path.join(webDir, 'src/locales/en.ts'),
    'paperless_ngxDescription',
    'English translation for description exists'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/locales/en.ts'),
    'paperless_ngxBaseUrlTip',
    'English translation for base URL tooltip exists'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/locales/en.ts'),
    'paperless_ngxApiTokenTip',
    'English translation for API token tooltip exists'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/locales/en.ts'),
    'paperless_ngxVerifySslTip',
    'English translation for verify SSL tooltip exists'
  );

  allPassed &= checkFileContains(
    path.join(webDir, 'src/locales/en.ts'),
    'paperless_ngxBatchSizeTip',
    'English translation for batch size tooltip exists'
  );

  // Summary
  log('\n=== Validation Summary ===\n', 'yellow');
  if (allPassed) {
    log('✓ All checks passed! The Paperless-ngx UI integration is complete.', 'green');
    log('\nNext steps:', 'yellow');
    log('1. Run `npm run dev` to start the development server');
    log('2. Navigate to Settings → Data Sources');
    log('3. Verify Paperless-ngx appears in the list');
    log('4. Test creating a new Paperless-ngx connector');
    process.exit(0);
  } else {
    log('✗ Some checks failed. Please review the errors above.', 'red');
    process.exit(1);
  }
}

// Run validation
runValidation();
