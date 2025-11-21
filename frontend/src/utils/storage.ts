/**
 * IndexedDB storage utility for user preferences
 */

const DB_NAME = 'ClaudeProxyPreferences';
const DB_VERSION = 1;
const STORE_NAME = 'preferences';

interface Preference {
  key: string;
  value: any;
  updatedAt: string;
}

/**
 * Initialize IndexedDB
 */
const initDB = (): Promise<IDBDatabase> => {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);

    request.onerror = () => {
      reject(request.error);
    };

    request.onsuccess = () => {
      resolve(request.result);
    };

    request.onupgradeneeded = (event) => {
      const db = (event.target as IDBOpenDBRequest).result;
      
      // Create object store if it doesn't exist
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME, { keyPath: 'key' });
      }
    };
  });
};

/**
 * Set a preference value
 */
export const setPreference = async (key: string, value: any): Promise<void> => {
  try {
    const db = await initDB();
    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);

    const preference: Preference = {
      key,
      value,
      updatedAt: new Date().toISOString(),
    };

    store.put(preference);

    return new Promise((resolve, reject) => {
      transaction.oncomplete = () => {
        db.close();
        resolve();
      };
      transaction.onerror = () => {
        db.close();
        reject(transaction.error);
      };
    });
  } catch (error) {
    console.error('Failed to set preference:', error);
    throw error;
  }
};

/**
 * Get a preference value
 */
export const getPreference = async <T = any>(key: string, defaultValue?: T): Promise<T | undefined> => {
  try {
    const db = await initDB();
    const transaction = db.transaction([STORE_NAME], 'readonly');
    const store = transaction.objectStore(STORE_NAME);
    const request = store.get(key);

    return new Promise((resolve, reject) => {
      request.onsuccess = () => {
        db.close();
        const result = request.result as Preference | undefined;
        resolve(result ? result.value : defaultValue);
      };
      request.onerror = () => {
        db.close();
        reject(request.error);
      };
    });
  } catch (error) {
    console.error('Failed to get preference:', error);
    return defaultValue;
  }
};

/**
 * Remove a preference
 */
export const removePreference = async (key: string): Promise<void> => {
  try {
    const db = await initDB();
    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    store.delete(key);

    return new Promise((resolve, reject) => {
      transaction.oncomplete = () => {
        db.close();
        resolve();
      };
      transaction.onerror = () => {
        db.close();
        reject(transaction.error);
      };
    });
  } catch (error) {
    console.error('Failed to remove preference:', error);
    throw error;
  }
};

/**
 * Clear all preferences
 */
export const clearAllPreferences = async (): Promise<void> => {
  try {
    const db = await initDB();
    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    store.clear();

    return new Promise((resolve, reject) => {
      transaction.oncomplete = () => {
        db.close();
        resolve();
      };
      transaction.onerror = () => {
        db.close();
        reject(transaction.error);
      };
    });
  } catch (error) {
    console.error('Failed to clear preferences:', error);
    throw error;
  }
};

/**
 * Preference keys
 */
export const PREFERENCE_KEYS = {
  CONFIG_LIST_VIEW_MODE: 'config_list_view_mode',
  CONFIG_LIST_SORT_FIELD: 'config_list_sort_field',
  CONFIG_LIST_SORT_ORDER: 'config_list_sort_order',
} as const;
