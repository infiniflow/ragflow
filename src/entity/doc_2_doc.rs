use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "doc_2_doc")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub parent_id: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub did: i64,
}

#[derive(Debug, Clone, Copy, EnumIter)]
pub enum Relation {
    Parent,
    Child
}

impl RelationTrait for Relation {
    fn def(&self) -> RelationDef {
        match self {
            Self::Parent => Entity::belongs_to(super::doc_info::Entity)
                .from(Column::ParentId)
                .to(super::doc_info::Column::Did)
                .into(),
            Self::Child => Entity::belongs_to(super::doc_info::Entity)
                .from(Column::Did)
                .to(super::doc_info::Column::Did)
                .into(),
        }
    }
}

impl ActiveModelBehavior for ActiveModel {}