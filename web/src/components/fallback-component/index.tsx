import React from 'react';
import { useTranslation } from 'react-i18next';

interface FallbackComponentProps {
  error?: Error;
  reset?: () => void;
}

const FallbackComponent: React.FC<FallbackComponentProps> = ({
  error,
  reset,
}) => {
  const { t } = useTranslation();

  return (
    <div style={{ padding: '20px', textAlign: 'center' }}>
      <h2>{t('error_boundary.title', 'Something went wrong')}</h2>
      <p>
        {t(
          'error_boundary.description',
          'Sorry, an error occurred while loading the page.',
        )}
      </p>
      {error && (
        <details style={{ whiteSpace: 'pre-wrap', marginTop: '16px' }}>
          <summary>{t('error_boundary.details', 'Error details')}</summary>
          {error.toString()}
        </details>
      )}
      <div style={{ marginTop: '16px' }}>
        <button
          onClick={() => window.location.reload()}
          style={{
            marginRight: '12px',
            padding: '8px 16px',
            backgroundColor: '#1890ff',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          {t('error_boundary.reload', 'Reload Page')}
        </button>
        {reset && (
          <button
            onClick={reset}
            style={{
              padding: '8px 16px',
              backgroundColor: '#52c41a',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            {t('error_boundary.retry', 'Retry')}
          </button>
        )}
      </div>
    </div>
  );
};

export default FallbackComponent;
