// Copyright 2016 Documize Inc. <legal@documize.com>. All rights reserved.
//
// This software (Documize Community Edition) is licensed under
// GNU AGPL v3 http://www.gnu.org/licenses/agpl-3.0.en.html
//
// You can operate outside the AGPL restrictions by purchasing
// Documize Enterprise Edition and obtaining a commercial license
// by contacting <sales@documize.com>.
//
// https://documize.com

import $ from 'jquery';
import { A } from '@ember/array';
import { inject as service } from '@ember/service';
import TooltipMixin from '../../mixins/tooltip';
import ModalMixin from '../../mixins/modal';
import Component from '@ember/component';

export default Component.extend(ModalMixin, TooltipMixin, {
	spaceSvc: service('folder'),
	groupSvc: service('group'),
	categorySvc: service('category'),
	appMeta: service(),
	store: service(),
	newCategory: '',
	deleteId: '',
	dropdown: null,

	init() {
		this._super(...arguments);
		this.users = [];
	},

	didReceiveAttrs() {
		this._super(...arguments);
		this.renderTooltips();
		this.load();
	},

	willDestroyElement() {
		this._super(...arguments);
		this.removeTooltips();
	},

	load() {
		// get categories
		this.get('categorySvc').getAll(this.get('folder.id')).then((c) => {
			this.set('category', c);

			// get summary of documents and users for each category in space
			this.get('categorySvc').getSummary(this.get('folder.id')).then((s) => {
				c.forEach((cat) => {
					let docs = _.where(s, {categoryId: cat.get('id'), type: 'documents'});
					let docCount = 0;
					docs.forEach((d) => { docCount = docCount + d.count });

					let users = _.where(s, {categoryId: cat.get('id'), type: 'users'});
					let userCount = 0;
					users.forEach((u) => { userCount = userCount + u.count });

					cat.set('documents', docCount);
					cat.set('users', userCount);
				});
			});
		});
	},

	permissionRecord(who, whoId, name) {
		let raw = {
			id: whoId,
			orgId: this.get('folder.orgId'),
			categoryId: this.get('currentCategory.id'),
			whoId: whoId,
			who: who,
			name: name,
			categoryView: false,
		};

		let rec = this.get('store').normalize('category-permission', raw);
		return this.get('store').push(rec);
	},

	setEdit(id, val) {
		let cats = this.get('category');
		let cat = cats.findBy('id', id);

		if (is.not.undefined(cat)) {
			cat.set('editMode', val);
		}

		return cat;
	},

	actions: {
		onAdd(e) {
			e.preventDefault();

			let cat = this.get('newCategory');

			if (cat === '') {
				$('#new-category-name').addClass('is-invalid').focus();
				return;
			}

			$('#new-category-name').removeClass('is-invalid').focus();
			this.set('newCategory', '');

			let c = {
				category: cat,
				folderId: this.get('folder.id')
			};

			this.get('categorySvc').add(c).then(() => {
				this.load();
			});
		},

		onShowDelete(id) {
			let cat = this.get('category').findBy('id', id);
			this.set('deleteId', cat.get('id'));

			this.modalOpen('#category-delete-modal', {show: true});
		},

		onDelete() {
			this.modalClose('#category-delete-modal');

			this.get('categorySvc').delete(this.get('deleteId')).then(() => {
				this.load();
			});
		},

		onEdit(id) {
			this.setEdit(id, true);
			this.removeTooltips();
		},

		onEditCancel(id) {
			this.setEdit(id, false);
			this.load();
			this.renderTooltips();
		},

		onSave(id) {
			let cat = this.setEdit(id, true);
			if (cat.get('category') === '') {
				$('#edit-category-' + cat.get('id')).addClass('is-invalid').focus();
				return false;
			}

			cat = this.setEdit(id, false);
			$('#edit-category-' + cat.get('id')).removeClass('is-invalid');

			this.get('categorySvc').save(cat).then(() => {
				this.load();
			});

			this.renderTooltips();
		},

		onShowAccessPicker(catId) {
			this.set('showCategoryAccess', true);

			let categoryPermissions = A([]);
			let category = this.get('category').findBy('id', catId);

			this.set('currentCategory', category);
			this.set('categoryPermissions', categoryPermissions);

			// get space permissions
			this.get('spaceSvc').getPermissions(this.get('folder.id')).then((spacePermissions) => {
				spacePermissions.forEach((sp) => {
					let cp  = this.permissionRecord(sp.get('who'), sp.get('whoId'), sp.get('name'));
					cp.set('selected', false);
					categoryPermissions.pushObject(cp);
				});

				this.get('categorySvc').getPermissions(category.get('id')).then((perms) => {
					// mark those users as selected that have permission to see the current category
					perms.forEach((perm) => {
						let c = categoryPermissions.findBy('whoId', perm.get('whoId'));
						if (is.not.undefined(c)) {
							c.set('selected', true);
						}
					});

					this.set('categoryPermissions', categoryPermissions.sortBy('who', 'name'));
				});
			});
		},

		onToggle(item) {
			item.set('selected', !item.get('selected'));
		},

		onGrantAccess() {
			this.set('showCategoryAccess', false);

			let folder = this.get('folder');
			let category = this.get('currentCategory');
			let perms = this.get('categoryPermissions').filterBy('selected', true);

			this.get('categorySvc').setViewers(folder.get('id'), category.get('id'), perms).then(() => {
				this.load();
			});
		}
	}
});
