'use-strict'

document.body.addEventListener('htmx:responseError', responseError)

// type definitions for htmx events.
/**
 * @typedef {Object} Xhr
 * @property {string} statusText
 * @property {number} status
 * @property {string} response - response as the server sends it.
 */
/**
 * @typedef {Object} Detail
 * @property {Xhr} xhr
 */
/**
 *  @typedef {Object} Event
 *  @property {Detail} detail
 */

/**
 * @param {Event} e 
 */
function responseError(e) {
    const status = e.detail.xhr.status;

    if (status === 401 || status === 403) {
        window.alert('You are not authorized to access this page or your session has expired. Please login again.');
        window.location = '/login' 
    }
}